package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	"github.com/elearning/user-service/internal/metrics"
	"github.com/elearning/user-service/internal/middleware"
)

const bcryptCost = 12

func (s *State) validatePasswordPolicy(r *http.Request, password string) (string, bool) {
	ctx := r.Context()

	minLen, _ := strconv.Atoi(ReadSetting(ctx, s.Pool, "password_min_length", "8"))
	if minLen < 1 {
		minLen = 8
	}
	if len(password) < minLen {
		return "Password must be at least " + strconv.Itoa(minLen) + " characters", false
	}
	if ReadSetting(ctx, s.Pool, "password_require_uppercase", "false") == "true" {
		hasUpper := false
		for _, c := range password {
			if unicode.IsUpper(c) {
				hasUpper = true
				break
			}
		}
		if !hasUpper {
			return "Password must contain at least one uppercase letter", false
		}
	}
	if ReadSetting(ctx, s.Pool, "password_require_number", "false") == "true" {
		hasNum := false
		for _, c := range password {
			if unicode.IsDigit(c) {
				hasNum = true
				break
			}
		}
		if !hasNum {
			return "Password must contain at least one number", false
		}
	}
	return "", true
}

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userPublicRow struct {
	ID           string  `json:"id"`
	Username     string  `json:"username"`
	Email        string  `json:"email"`
	Role         string  `json:"role"`
	AvatarURL    *string `json:"avatar_url"`
	Bio          *string `json:"bio"`
	IsActive     bool    `json:"is_active"`
	AuthProvider string  `json:"auth_provider"`
	CreatedAt    string  `json:"created_at"`
}

type authResponse struct {
	Token string        `json:"token"`
	User  userPublicRow `json:"user"`
}

func scanUserPublic(row interface {
	Scan(...any) error
}) (userPublicRow, error) {
	var u userPublicRow
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.Role,
		&u.AvatarURL, &u.Bio, &u.IsActive, &u.AuthProvider, &u.CreatedAt)
	return u, err
}

// Register godoc
// @Summary  Register a new user
// @Tags     Auth
// @Accept   json
// @Produce  json
// @Param    body  body  registerRequest  true  "Registration details"
// @Success  200   {object}  authResponse
// @Failure  400   {object}  map[string]string
// @Failure  409   {object}  map[string]string
// @Router   /api/auth/register [post]
func (s *State) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	ctx := r.Context()

	if ReadSetting(ctx, s.Pool, "registration_enabled", "true") != "true" {
		s.Error(w, http.StatusForbidden, "New user registration is currently disabled.")
		return
	}

	whitelist := ReadSetting(ctx, s.Pool, "registration_email_whitelist", "")
	if strings.TrimSpace(whitelist) != "" {
		allowed := strings.Split(whitelist, ",")
		domain := ""
		if parts := strings.SplitN(req.Email, "@", 2); len(parts) == 2 {
			domain = parts[1]
		}
		ok := false
		for _, d := range allowed {
			if strings.EqualFold(strings.TrimSpace(d), domain) {
				ok = true
				break
			}
		}
		if !ok {
			s.Error(w, http.StatusForbidden, "Registration is restricted to specific email domains.")
			return
		}
	}

	if len(req.Username) < 3 {
		s.Error(w, http.StatusBadRequest, "Username must be at least 3 characters")
		return
	}

	if msg, ok := s.validatePasswordPolicy(r, req.Password); !ok {
		s.Error(w, http.StatusBadRequest, msg)
		return
	}

	var existing int64
	err := s.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM users WHERE email = $1 OR username = $2",
		req.Email, req.Username).Scan(&existing)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	if existing > 0 {
		s.Error(w, http.StatusConflict, "Email or username already taken")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Hash error")
		return
	}

	u, err := scanUserPublic(s.Pool.QueryRow(ctx,
		`INSERT INTO users (username, email, password_hash, role)
		 VALUES ($1, $2, $3, 'student')
		 RETURNING id::text, username, email, role, avatar_url, bio, is_active, auth_provider, created_at::text`,
		req.Username, req.Email, string(hash)))
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}

	token, err := middleware.CreateToken(u.ID, u.Email, u.Role, s.Config.JWTSecret, s.Config.JWTExpiryH)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Token error")
		return
	}

	addToDefaultGroup(ctx, s.Pool, u.ID)
	syncGroupEnrollments(ctx, s.Pool, u.ID)
	metrics.ActiveUsers.Inc()
	s.JSON(w, http.StatusOK, authResponse{Token: token, User: u})
}

// Login godoc
// @Summary  Authenticate and receive a JWT
// @Tags     Auth
// @Accept   json
// @Produce  json
// @Param    body  body  loginRequest  true  "Login credentials"
// @Success  200   {object}  authResponse
// @Failure  401   {object}  map[string]string
// @Router   /api/auth/login [post]
func (s *State) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decode(r, &req); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	ctx := r.Context()

	if ReadSetting(ctx, s.Pool, "sso_local_login_enabled", "true") != "true" {
		s.Error(w, http.StatusForbidden, "Local login is disabled. Please sign in using an SSO provider.")
		return
	}

	var id, username, email, role, authProvider string
	var passwordHash, avatarURL, bio *string
	var isActive bool
	var createdAt string
	err := s.Pool.QueryRow(ctx,
		`SELECT id::text, username, email, password_hash, role, avatar_url, bio, is_active, auth_provider, created_at::text
		 FROM users WHERE email = $1 AND is_active = TRUE`, req.Email).
		Scan(&id, &username, &email, &passwordHash, &role, &avatarURL, &bio, &isActive, &authProvider, &createdAt)
	if err != nil {
		s.Error(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}
	if passwordHash == nil {
		s.Error(w, http.StatusUnauthorized, "This account uses "+authProvider+" SSO login.")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*passwordHash), []byte(req.Password)); err != nil {
		s.Error(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}

	token, err := middleware.CreateToken(id, email, role, s.Config.JWTSecret, s.Config.JWTExpiryH)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Token error")
		return
	}

	addToDefaultGroup(ctx, s.Pool, id)
	syncGroupEnrollments(ctx, s.Pool, id)
	s.JSON(w, http.StatusOK, authResponse{
		Token: token,
		User: userPublicRow{
			ID: id, Username: username, Email: email, Role: role,
			AvatarURL: avatarURL, Bio: bio, IsActive: isActive,
			AuthProvider: authProvider, CreatedAt: createdAt,
		},
	})
}

// Me godoc
// @Summary   Get current user profile
// @Tags      Auth
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  userPublicRow
// @Failure   404  {object}  map[string]string
// @Router    /api/auth/me [get]
func (s *State) Me(w http.ResponseWriter, r *http.Request) {
	c := s.claims(r)
	u, err := scanUserPublic(s.Pool.QueryRow(r.Context(),
		`SELECT id::text, username, email, role, avatar_url, bio, is_active, auth_provider, created_at::text
		 FROM users WHERE id = $1::uuid`, c.Subject))
	if err != nil {
		s.Error(w, http.StatusNotFound, "User not found")
		return
	}
	s.JSON(w, http.StatusOK, u)
}

// UpdateProfile godoc
// @Summary   Update current user profile
// @Tags      Auth
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     body  body  object  true  "Fields: username, bio, avatar_url (all optional)"
// @Success   200   {object}  userPublicRow
// @Failure   400   {object}  map[string]string
// @Router    /api/auth/profile [put]
func (s *State) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username  *string `json:"username"`
		Bio       *string `json:"bio"`
		AvatarURL *string `json:"avatar_url"`
	}
	if err := decode(r, &body); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	ctx := r.Context()
	c := s.claims(r)

	if body.Username != nil {
		uname := strings.TrimSpace(*body.Username)
		if uname != "" {
			if ReadSetting(ctx, s.Pool, "profile_allow_username_change", "true") != "true" {
				s.Error(w, http.StatusForbidden, "Username changes are not allowed.")
				return
			}
			if len(uname) < 3 {
				s.Error(w, http.StatusBadRequest, "Username must be at least 3 characters")
				return
			}
			var taken int64
			_ = s.Pool.QueryRow(ctx,
				"SELECT COUNT(*) FROM users WHERE username = $1 AND id != $2::uuid",
				uname, c.Subject).Scan(&taken)
			if taken > 0 {
				s.Error(w, http.StatusConflict, "Username already taken")
				return
			}
		}
	}

	u, err := scanUserPublic(s.Pool.QueryRow(ctx,
		`UPDATE users SET
			username   = COALESCE($1, username),
			bio        = COALESCE($2, bio),
			avatar_url = COALESCE($3, avatar_url),
			updated_at = NOW()
		 WHERE id = $4::uuid
		 RETURNING id::text, username, email, role, avatar_url, bio, is_active, auth_provider, created_at::text`,
		body.Username, body.Bio, body.AvatarURL, c.Subject))
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.JSON(w, http.StatusOK, u)
}

// ChangePassword godoc
// @Summary   Change the current user's password
// @Tags      Auth
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     body  body  object  true  "old_password and new_password"
// @Success   200   {object}  map[string]string
// @Failure   400   {object}  map[string]string
// @Failure   401   {object}  map[string]string
// @Router    /api/auth/password [put]
func (s *State) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := decode(r, &body); err != nil {
		s.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if body.OldPassword == "" {
		s.Error(w, http.StatusBadRequest, "old_password required")
		return
	}
	if body.NewPassword == "" {
		s.Error(w, http.StatusBadRequest, "new_password required")
		return
	}
	if msg, ok := s.validatePasswordPolicy(r, body.NewPassword); !ok {
		s.Error(w, http.StatusBadRequest, msg)
		return
	}

	ctx := r.Context()
	c := s.claims(r)

	var passwordHash *string
	err := s.Pool.QueryRow(ctx,
		"SELECT password_hash FROM users WHERE id = $1::uuid", c.Subject).Scan(&passwordHash)
	if err != nil || passwordHash == nil {
		s.Error(w, http.StatusBadRequest, "This account uses SSO login and has no local password.")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*passwordHash), []byte(body.OldPassword)); err != nil {
		s.Error(w, http.StatusUnauthorized, "Incorrect current password")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcryptCost)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Hash error")
		return
	}
	_, err = s.Pool.Exec(ctx,
		"UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2::uuid",
		string(newHash), c.Subject)
	if err != nil {
		s.Error(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.JSON(w, http.StatusOK, map[string]string{"message": "Password changed successfully"})
}
