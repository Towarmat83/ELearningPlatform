package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"github.com/gorilla/websocket"

	"github.com/elearning/api-go/internal/middleware"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// POST /api/courses/{course_id}/labs/{lab_id}/instance
func (s *State) StartInstance(w http.ResponseWriter, r *http.Request) {
	if s.Docker == nil {
		s.Error(w, http.StatusBadRequest, "Interactive labs are not available (Docker not connected)")
		return
	}
	courseID := param(r, "course_id")
	labID := param(r, "lab_id")
	c := s.claims(r)
	ctx := r.Context()

	if c.Role != "admin" {
		var enrolled int64
		s.Pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM enrollments WHERE user_id = $1::uuid AND course_id = $2::uuid",
			c.Subject, courseID).Scan(&enrolled)
		if enrolled == 0 {
			s.Error(w, http.StatusForbidden, "You must enroll in this course first")
			return
		}
	}

	var contentStr string
	if err := s.Pool.QueryRow(ctx,
		"SELECT content::text FROM labs WHERE id = $1::uuid AND course_id = $2::uuid",
		labID, courseID).Scan(&contentStr); err != nil {
		s.Error(w, http.StatusNotFound, "Lab not found")
		return
	}
	var contentMap map[string]any
	json.Unmarshal([]byte(contentStr), &contentMap) //nolint:errcheck
	dockerImage, _ := contentMap["docker_image"].(string)
	if dockerImage == "" {
		s.Error(w, http.StatusBadRequest, "This lab has no interactive environment configured")
		return
	}

	// Return existing running container if alive
	var prevInstanceID, prevContainerID, prevStatus string
	err := s.Pool.QueryRow(ctx,
		"SELECT id::text, container_id, status FROM lab_instances WHERE user_id = $1::uuid AND lab_id = $2::uuid",
		c.Subject, labID).Scan(&prevInstanceID, &prevContainerID, &prevStatus)
	if err == nil && prevStatus == "running" {
		if _, err := s.Docker.ContainerInspect(ctx, prevContainerID); err == nil {
			var expiresAt time.Time
			s.Pool.QueryRow(ctx, "SELECT expires_at FROM lab_instances WHERE id = $1::uuid", prevInstanceID).Scan(&expiresAt)
			s.JSON(w, http.StatusOK, map[string]any{
				"instance_id": prevInstanceID, "status": "running", "expires_at": expiresAt,
			})
			return
		}
		// Stale — remove
		s.Docker.ContainerRemove(ctx, prevContainerID, container.RemoveOptions{Force: true}) //nolint:errcheck
	}

	// Pull image if not locally present
	if _, _, err := s.Docker.ImageInspectWithRaw(ctx, dockerImage); err != nil {
		slog.Info("pulling docker image", "image", dockerImage)
		pullReader, pullErr := s.Docker.ImagePull(ctx, dockerImage, image.PullOptions{})
		if pullErr != nil {
			s.Error(w, http.StatusInternalServerError, "Failed to pull image: "+pullErr.Error())
			return
		}
		io.Copy(io.Discard, pullReader) //nolint:errcheck
		pullReader.Close()
	}

	// Create container
	containerName := "lab-" + c.Subject[:8] + "-" + labID[:8]
	s.Docker.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true}) //nolint:errcheck

	mem := int64(512 * 1024 * 1024)
	nanoCPU := int64(500_000_000)
	pidsLimit := int64(50)
	createResp, err := s.Docker.ContainerCreate(ctx,
		&container.Config{Image: dockerImage, Tty: true, OpenStdin: true},
		&container.HostConfig{
			NetworkMode: "none",
			Resources: container.Resources{
				Memory:    mem,
				NanoCPUs:  nanoCPU,
				PidsLimit: &pidsLimit,
			},
		},
		nil, nil, containerName)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to create container: "+err.Error())
		return
	}
	if err := s.Docker.ContainerStart(ctx, createResp.ID, container.StartOptions{}); err != nil {
		s.Error(w, http.StatusInternalServerError, "Failed to start container: "+err.Error())
		return
	}

	// Run init_script if provided (sets up challenge files, flags, etc.)
	if initScript, _ := contentMap["init_script"].(string); initScript != "" {
		if err := runInitScript(ctx, s.Docker, createResp.ID, initScript); err != nil {
			slog.Error("init_script failed", "container", createResp.ID, "err", err)
			// Non-fatal: log but continue — student still gets a terminal
		}
	}

	var newInstanceID string
	var newExpiresAt time.Time
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO lab_instances (user_id, lab_id, container_id, status, started_at, expires_at)
		VALUES ($1::uuid, $2::uuid, $3, 'running', NOW(), NOW() + INTERVAL '30 minutes')
		ON CONFLICT (user_id, lab_id) DO UPDATE
			SET container_id = $3, status = 'running',
			    started_at = NOW(), expires_at = NOW() + INTERVAL '30 minutes'
		RETURNING id::text, expires_at`,
		c.Subject, labID, createResp.ID).Scan(&newInstanceID, &newExpiresAt)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]any{
		"instance_id": newInstanceID, "status": "running", "expires_at": newExpiresAt,
	})
}

// GET /api/courses/{course_id}/labs/{lab_id}/instance
func (s *State) GetInstance(w http.ResponseWriter, r *http.Request) {
	labID := param(r, "lab_id")
	c := s.claims(r)

	var id, status string
	var startedAt, expiresAt time.Time
	err := s.Pool.QueryRow(r.Context(),
		"SELECT id::text, status, started_at, expires_at FROM lab_instances WHERE user_id = $1::uuid AND lab_id = $2::uuid",
		c.Subject, labID).Scan(&id, &status, &startedAt, &expiresAt)
	if err != nil {
		s.JSON(w, http.StatusOK, map[string]string{"status": "none"})
		return
	}
	s.JSON(w, http.StatusOK, map[string]any{
		"instance_id": id, "status": status, "started_at": startedAt, "expires_at": expiresAt,
	})
}

// DELETE /api/courses/{course_id}/labs/{lab_id}/instance
func (s *State) StopInstance(w http.ResponseWriter, r *http.Request) {
	if s.Docker == nil {
		s.Error(w, http.StatusBadRequest, "Docker not available")
		return
	}
	labID := param(r, "lab_id")
	c := s.claims(r)
	ctx := r.Context()

	var instanceID, containerID string
	if err := s.Pool.QueryRow(ctx,
		"SELECT id::text, container_id FROM lab_instances WHERE user_id = $1::uuid AND lab_id = $2::uuid AND status = 'running'",
		c.Subject, labID).Scan(&instanceID, &containerID); err != nil {
		s.Error(w, http.StatusNotFound, "No running instance found")
		return
	}
	s.Docker.ContainerStop(ctx, containerID, container.StopOptions{})               //nolint:errcheck
	s.Docker.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}) //nolint:errcheck
	s.Pool.Exec(ctx, "UPDATE lab_instances SET status = 'stopped' WHERE id = $1::uuid", instanceID) //nolint:errcheck
	s.JSON(w, http.StatusOK, map[string]string{"message": "Instance stopped"})
}

// GET /ws/courses/{course_id}/labs/{lab_id}/terminal?token=<JWT>
func (s *State) TerminalWS(w http.ResponseWriter, r *http.Request) {
	if s.Docker == nil {
		http.Error(w, `{"error":"Docker not available"}`, http.StatusBadRequest)
		return
	}
	labID := param(r, "lab_id")
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, `{"error":"Missing token query parameter"}`, http.StatusUnauthorized)
		return
	}
	claims, err := middleware.VerifyToken(tokenStr, s.Config.JWTSecret)
	if err != nil {
		http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	var containerID string
	if err := s.Pool.QueryRow(ctx,
		"SELECT container_id FROM lab_instances WHERE user_id = $1::uuid AND lab_id = $2::uuid AND status = 'running'",
		claims.Subject, labID).Scan(&containerID); err != nil {
		http.Error(w, `{"error":"No running instance — start one first"}`, http.StatusNotFound)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	if err := runTerminal(ctx, conn, s.Docker, containerID); err != nil {
		slog.Error("terminal error", "err", err)
	}
}

type resizeMsg struct {
	Type string `json:"type"`
	Rows uint   `json:"rows"`
	Cols uint   `json:"cols"`
}

func runTerminal(ctx context.Context, conn *websocket.Conn, docker *dockerclient.Client, containerID string) error {
	// Create a TTY exec inside the running container
	execResp, err := docker.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          []string{"/bin/sh"},
	})
	if err != nil {
		return err
	}

	// Attach to exec (returns a HijackedResponse with Reader + Conn)
	hijack, err := docker.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{Tty: true})
	if err != nil {
		return err
	}
	defer hijack.Close()

	// Goroutine: Docker stdout/stderr → WebSocket client
	fwdDone := make(chan struct{})
	go func() {
		defer close(fwdDone)
		buf := make([]byte, 4096)
		for {
			n, err := hijack.Reader.Read(buf)
			if n > 0 {
				if writeErr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Main loop: WebSocket → Docker stdin (resize messages handled inline)
	for {
		select {
		case <-fwdDone:
			return nil
		default:
		}

		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		switch msgType {
		case websocket.BinaryMessage:
			if _, err := hijack.Conn.Write(msg); err != nil {
				return err
			}
		case websocket.TextMessage:
			var rm resizeMsg
			if json.Unmarshal(msg, &rm) == nil && rm.Type == "resize" {
				docker.ContainerExecResize(ctx, execResp.ID, container.ResizeOptions{ //nolint:errcheck
					Height: rm.Rows, Width: rm.Cols,
				})
			} else {
				if _, err := hijack.Conn.Write(msg); err != nil {
					return err
				}
			}
		case websocket.CloseMessage:
			return nil
		}
	}
	return nil
}

// runInitScript executes a shell script inside a running container to set up the challenge.
func runInitScript(ctx context.Context, docker *dockerclient.Client, containerID, script string) error {
	execResp, err := docker.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"/bin/sh", "-c", script},
	})
	if err != nil {
		return err
	}
	resp, err := docker.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return err
	}
	defer resp.Close()
	io.Copy(io.Discard, resp.Reader) //nolint:errcheck
	return nil
}
