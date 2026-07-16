package db

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const (
	// defaultAdminEmail is the email of the bootstrapped admin account.
	defaultAdminEmail = "admin@pupitre.local"
	// defaultAdminUsername is the username of the bootstrapped admin account.
	defaultAdminUsername = "admin"
	// defaultAdminHash is the bcrypt hash of "Admin@1234" (cost 12).
	// Used only as a last-resort fallback when no ADMIN_PASSWORD is configured.
	defaultAdminHash = "$2y$12$U6BVYjCKzHaIu2VrJNHDhuBUNTiOrcP0xoovwKbGSvOMd29qwZz.y"
	// bcryptCost is the work factor used when hashing a cleartext password.
	bcryptCost = 12
	// adminPasswordPollInterval is how often WatchAdminPassword rereads the
	// admin password file for changes.
	adminPasswordPollInterval = 30 * time.Second
)

// WatchAdminPassword polls filePath every 30 s and calls SeedAdmin whenever
// the file content changes.  It is designed to work with Kubernetes Secret
// volumes: the kubelet refreshes the mounted files automatically (within ~60 s)
// when the Secret is updated, so no pod restart is needed to rotate the
// admin password.
//
// The goroutine exits cleanly when ctx is cancelled (graceful shutdown).
func WatchAdminPassword(ctx context.Context, pool *pgxpool.Pool, filePath string) {
	ticker := time.NewTicker(adminPasswordPollInterval)
	defer ticker.Stop()

	// Capture the current file content so we only act on real changes.
	last, _ := os.ReadFile(filePath) //nolint:gosec // path is operator-controlled via ADMIN_PASSWORD_FILE, not user input

	zap.L().Info("admin password watcher started", zap.String("file", filePath))

	for {
		select {
		case <-ctx.Done():
			zap.L().Info("admin password watcher stopped")

			return
		case <-ticker.C:
			current, err := os.ReadFile(filePath) //nolint:gosec // path is operator-controlled via ADMIN_PASSWORD_FILE, not user input
			if err != nil {
				zap.L().Warn("admin password file unreadable", zap.String("path", filePath), zap.Error(err))

				continue
			}

			if bytes.Equal(current, last) {
				continue
			}

			last = current
			password := strings.TrimSpace(string(current))

			zap.L().Info("admin password file changed — re-seeding admin account")

			err = SeedAdmin(ctx, pool, password)
			if err != nil {
				zap.L().Error("re-seed admin failed", zap.Error(err))
			} else {
				zap.L().Info("admin password updated successfully (no restart required)")
			}
		}
	}
}

// SeedAdmin ensures the default admin user exists and carries the configured
// password hash.  It is called once at startup, after migrations.
//
// Resolution order for the password:
//  1. If adminPassword starts with "$2" it is treated as a pre-computed bcrypt
//     hash and used directly (allows secrets to store either a plaintext or a
//     pre-hashed value under the same ADMIN_PASSWORD key).
//  2. If adminPassword is a non-empty plaintext string it is bcrypt-hashed
//     (cost 12) before being stored.
//  3. If adminPassword is empty the hardcoded default hash is used and a loud
//     warning is logged — intended only for local development.
//
// Behaviour on conflict (admin email already exists):
//   - If a custom password was provided (cases 1 or 2) the hash is always
//     updated so deployers can rotate the credential by restarting the pod.
//   - If no password was provided (case 3) the existing row is left untouched
//     so a previously changed password is not overwritten.
func SeedAdmin(ctx context.Context, pool *pgxpool.Pool, adminPassword string) error {
	var hash string

	useDefault := false

	switch {
	case strings.HasPrefix(adminPassword, "$2"):
		// Pre-computed bcrypt hash — use as-is.
		hash = adminPassword

		zap.L().Info("admin seeding: using pre-computed bcrypt hash from ADMIN_PASSWORD")

	case adminPassword != "":
		// Cleartext password — hash it now.
		h, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcryptCost)
		if err != nil {
			return fmt.Errorf("hash admin password: %w", err)
		}

		hash = string(h)

		zap.L().Info("admin seeding: hashed ADMIN_PASSWORD, updating admin account")

	default:
		// No password configured — fall back to the hardcoded default.
		hash = defaultAdminHash
		useDefault = true

		zap.L().Warn("ADMIN_PASSWORD is not set — using the default password 'Admin@1234'. " +
			"Set ADMIN_PASSWORD to a strong secret before going to production.")
	}

	if useDefault {
		// Only insert if no admin exists yet; never overwrite a potentially
		// customised password when running with the built-in fallback.
		_, err := pool.Exec(ctx, `
			INSERT INTO users (username, email, password_hash, role)
			VALUES ($1, $2, $3, 'admin')
			ON CONFLICT DO NOTHING`,
			defaultAdminUsername, defaultAdminEmail, hash)
		if err != nil {
			return fmt.Errorf("insert default admin: %w", err)
		}

		return nil
	}

	// Custom password: upsert so the hash is refreshed on every restart.
	// This lets operators rotate the password by updating the secret and
	// rolling the pod — no manual SQL required.
	_, err := pool.Exec(ctx, `
		INSERT INTO users (username, email, password_hash, role)
		VALUES ($1, $2, $3, 'admin')
		ON CONFLICT (email) DO UPDATE SET
			password_hash = EXCLUDED.password_hash,
			updatedAt    = NOW()`,
		defaultAdminUsername, defaultAdminEmail, hash)
	if err != nil {
		return fmt.Errorf("upsert admin password: %w", err)
	}

	return nil
}
