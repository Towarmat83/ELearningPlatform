package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	"github.com/elearning/user-service/internal/metrics"
	"github.com/elearning/user-service/internal/middleware"
)

// bcryptCost is the bcrypt hashing cost used for all stored passwords.
const bcryptCost = 12

// Settings/validation literals shared across this file, pulled out to
// satisfy goconst and mnd.
const (
	authSettingTrue         = "true"
	authEmailDomainParts    = 2
	authMinUsernameLen      = 3
	authMsgUsernameTooShort = "Username must be at least 3 characters"
)

// passwordHasUpper reports whether password contains an uppercase letter.
func passwordHasUpper(password string) bool {
	for _, char := range password {
		if unicode.IsUpper(char) {
			return true
		}
	}

	return false
}

// passwordHasDigit reports whether password contains a decimal digit.
func passwordHasDigit(password string) bool {
	for _, char := range password {
		if unicode.IsDigit(char) {
			return true
		}
	}

	return false
}

// validatePasswordPolicy checks password against the configured min
// length, uppercase, and digit requirements. It returns a rejection
// message and false, or an empty message and true when password is
// acceptable.
func (s *State) validatePasswordPolicy(request *http.Request, password string) (string, bool) {
	ctx := request.Context()

	minLen, _ := strconv.Atoi(ReadSetting(ctx, s.Pool, settingKeyPasswordMinLength, "8"))
	if minLen < 1 {
		minLen = 8
	}

	if len(password) < minLen {
		return "Password must be at least " + strconv.Itoa(minLen) + " characters", false
	}

	requireUpper := ReadSetting(ctx, s.Pool, settingKeyPasswordRequireUppercase, "false") == authSettingTrue
	if requireUpper && !passwordHasUpper(password) {
		return "Password must contain at least one uppercase letter", false
	}

	requireDigit := ReadSetting(ctx, s.Pool, settingKeyPasswordRequireNumber, "false") == authSettingTrue
	if requireDigit && !passwordHasDigit(password) {
		return "Password must contain at least one number", false
	}

	return "", true
}

// registerRequest is the JSON payload for POST /api/auth/register.
type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loginRequest is the JSON payload for POST /api/auth/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// userPublicRow is the public-facing representation of a user account,
// as returned by the auth and profile endpoints.
type userPublicRow struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`

	AvatarURL *string `json:"avatarUrl"`
	Bio       *string `json:"bio"`
	IsActive  bool    `json:"isActive"`

	AuthProvider string `json:"authProvider"`
	CreatedAt    string `json:"createdAt"`
}

// authResponse wraps a JWT and the authenticated user's public profile,
// as returned by register/login.
type authResponse struct {
	Token string        `json:"token"`
	User  userPublicRow `json:"user"`
}

// scanUserPublic scans a users-table row into a userPublicRow.
func scanUserPublic(row interface {
	Scan(dest ...any) error
}) (userPublicRow, error) {
	var user userPublicRow

	err := row.Scan(&user.ID, &user.Username, &user.Email, &user.Role,
		&user.AvatarURL, &user.Bio, &user.IsActive, &user.AuthProvider, &user.CreatedAt)
	if err != nil {
		return userPublicRow{}, fmt.Errorf("scan user row: %w", err)
	}

	return user, nil
}

// registrationAllowedForEmail reports whether email's domain is allowed
// by the registration_email_whitelist setting. An empty/unset whitelist
// permits every domain.
func (s *State) registrationAllowedForEmail(ctx context.Context, email string) bool {
	whitelist := ReadSetting(ctx, s.Pool, "registration_email_whitelist", "")
	if strings.TrimSpace(whitelist) == "" {
		return true
	}

	domain := ""

	parts := strings.SplitN(email, "@", authEmailDomainParts)
	if len(parts) == authEmailDomainParts {
		domain = parts[1]
	}

	for allowedDomain := range strings.SplitSeq(whitelist, ",") {
		if strings.EqualFold(strings.TrimSpace(allowedDomain), domain) {
			return true
		}
	}

	return false
}

// registerValidationCheck validates a registration request against
// platform settings and password policy. It returns the HTTP
// status/message to send back when registration must be rejected, or
// ok == true to proceed.
//
//nolint:gocritic // named results here would trip nonamedreturns instead; see doc comment above for the meaning of each value
func (s *State) registerValidationCheck(request *http.Request, req registerRequest) (int, string, bool) {
	ctx := request.Context()

	if ReadSetting(ctx, s.Pool, settingKeyRegistrationEnabled, authSettingTrue) != authSettingTrue {
		return http.StatusForbidden, "New user registration is currently disabled.", false
	}

	if !s.registrationAllowedForEmail(ctx, req.Email) {
		return http.StatusForbidden, "Registration is restricted to specific email domains.", false
	}

	if len(req.Username) < authMinUsernameLen {
		return http.StatusBadRequest, authMsgUsernameTooShort, false
	}

	if msg, ok := s.validatePasswordPolicy(request, req.Password); !ok {
		return http.StatusBadRequest, msg, false
	}

	return http.StatusOK, "", true
}

// emailOrUsernameTaken reports whether email or username already
// belongs to an existing account.
func (s *State) emailOrUsernameTaken(ctx context.Context, email, username string) (bool, error) {
	var count int64

	err := s.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM users WHERE email = $1 OR username = $2",
		email, username).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check existing user: %w", err)
	}

	return count > 0, nil
}

// createUser hashes req.Password and inserts a new user row, returning
// its public representation.
func (s *State) createUser(ctx context.Context, req registerRequest) (userPublicRow, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		return userPublicRow{}, fmt.Errorf("hash password: %w", err)
	}

	return scanUserPublic(s.Pool.QueryRow(ctx,
		`INSERT INTO users (username, email, password_hash, role)
		 VALUES ($1, $2, $3, 'student')
		 RETURNING id::text, username, email, role, avatarUrl, bio, isActive, authProvider, createdAt::text`,
		req.Username, req.Email, string(hash)))
}

// Register godoc
// @Summary  Register a new user
// @Tags     Auth
// @Accept   json
// @Produce  json
// @Param    body  registerRequest  true  "Registration details"
// @Success  200   {object}  authResponse
// @Failure  400   {object}  map[string]string
// @Failure  409   {object}  map[string]string
// @Router   /api/auth/register [post].
func (s *State) Register(writer http.ResponseWriter, request *http.Request) {
	var req registerRequest

	err := decode(request, &req)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	status, msg, ok := s.registerValidationCheck(request, req)
	if !ok {
		s.Error(writer, status, msg)

		return
	}

	ctx := request.Context()

	taken, err := s.emailOrUsernameTaken(ctx, req.Email, req.Username)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	if taken {
		s.Error(writer, http.StatusConflict, "Email or username already taken")

		return
	}

	user, err := s.createUser(ctx, req)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	token, err := middleware.CreateToken(user.ID, user.Email, user.Role, s.Config.JWTSecret, s.Config.JWTExpiryH)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Token error")

		return
	}

	addToDefaultGroup(ctx, s.Pool, user.ID)
	syncGroupEnrollments(ctx, s.Pool, user.ID)
	metrics.ActiveUsers.Inc()
	s.JSON(writer, http.StatusOK, authResponse{Token: token, User: user})
}

// loginUserRow holds a local-login lookup result: the user's public
// profile plus their password hash (nil for SSO-only accounts).
type loginUserRow struct {
	user         userPublicRow
	passwordHash *string
}

// loginLookup fetches the local-login row for email, including its
// password hash (nil for SSO-only accounts). Only active accounts match.
func (s *State) loginLookup(ctx context.Context, email string) (loginUserRow, error) {
	var row loginUserRow

	err := s.Pool.QueryRow(ctx,
		`SELECT id::text, username, email, password_hash, role, avatarUrl, bio, isActive, authProvider, createdAt::text
		 FROM users WHERE email = $1 AND isActive = TRUE`, email).
		Scan(&row.user.ID, &row.user.Username, &row.user.Email, &row.passwordHash, &row.user.Role,
			&row.user.AvatarURL, &row.user.Bio, &row.user.IsActive, &row.user.AuthProvider, &row.user.CreatedAt)
	if err != nil {
		return loginUserRow{}, fmt.Errorf("lookup local user: %w", err)
	}

	return row, nil
}

// Login godoc
// @Summary  Authenticate and receive a JWT
// @Tags     Auth
// @Accept   json
// @Produce  json
// @Param    body  loginRequest  true  "Login credentials"
// @Success  200   {object}  authResponse
// @Failure  401   {object}  map[string]string
// @Router   /api/auth/login [post].
func (s *State) Login(writer http.ResponseWriter, request *http.Request) {
	var req loginRequest

	err := decode(request, &req)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	ctx := request.Context()

	if ReadSetting(ctx, s.Pool, settingKeySSOLocalLoginEnabled, authSettingTrue) != authSettingTrue {
		s.Error(writer, http.StatusForbidden, "Local login is disabled. Please sign in using an SSO provider.")

		return
	}

	row, err := s.loginLookup(ctx, req.Email)
	if err != nil {
		s.Error(writer, http.StatusUnauthorized, "Invalid email or password")

		return
	}

	if row.passwordHash == nil {
		s.Error(writer, http.StatusUnauthorized, "This account uses "+row.user.AuthProvider+" SSO login.")

		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(*row.passwordHash), []byte(req.Password))
	if err != nil {
		s.Error(writer, http.StatusUnauthorized, "Invalid email or password")

		return
	}

	token, err := middleware.CreateToken(row.user.ID, row.user.Email, row.user.Role, s.Config.JWTSecret, s.Config.JWTExpiryH)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Token error")

		return
	}

	addToDefaultGroup(ctx, s.Pool, row.user.ID)
	syncGroupEnrollments(ctx, s.Pool, row.user.ID)
	s.JSON(writer, http.StatusOK, authResponse{Token: token, User: row.user})
}

// Me godoc
// @Summary   Get current user profile
// @Tags      Auth
// @Security  BearerAuth
// @Produce   json
// @Success   200  {object}  userPublicRow
// @Failure   404  {object}  map[string]string
// @Router    /api/auth/me [get].
func (s *State) Me(writer http.ResponseWriter, request *http.Request) {
	claims := s.claims(request)

	user, err := scanUserPublic(s.Pool.QueryRow(request.Context(),
		`SELECT id::text, username, email, role, avatarUrl, bio, isActive, authProvider, createdAt::text
		 FROM users WHERE id = $1::uuid`, claims.Subject))
	if err != nil {
		s.Error(writer, http.StatusNotFound, "User not found")

		return
	}

	s.JSON(writer, http.StatusOK, user)
}

// usernameChangeCheck validates a requested username change against
// platform settings and uniqueness. It returns the HTTP status/message
// to send back when the change must be rejected, or ok == true to
// proceed.
//
//nolint:gocritic // named results here would trip nonamedreturns instead; see doc comment above for the meaning of each value
func (s *State) usernameChangeCheck(ctx context.Context, uname, currentUserID string) (int, string, bool) {
	if ReadSetting(ctx, s.Pool, "profile_allow_username_change", authSettingTrue) != authSettingTrue {
		return http.StatusForbidden, "Username changes are not allowed.", false
	}

	if len(uname) < authMinUsernameLen {
		return http.StatusBadRequest, authMsgUsernameTooShort, false
	}

	var taken int64

	_ = s.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM users WHERE username = $1 AND id != $2::uuid",
		uname, currentUserID).Scan(&taken)
	if taken > 0 {
		return http.StatusConflict, "Username already taken", false
	}

	return http.StatusOK, "", true
}

// UpdateProfile godoc
// @Summary   Update current user profile
// @Tags      Auth
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     body  object  true  "username, bio, avatarUrl (all optional)"
// @Success   200   {object}  userPublicRow
// @Failure   400   {object}  map[string]string
// @Router    /api/auth/profile [put].
func (s *State) UpdateProfile(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Username *string `json:"username"`
		Bio      *string `json:"bio"`

		AvatarURL *string `json:"avatarUrl"`
	}

	err := decode(request, &body)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	ctx := request.Context()
	claims := s.claims(request)

	if body.Username != nil {
		uname := strings.TrimSpace(*body.Username)
		if uname != "" {
			status, msg, ok := s.usernameChangeCheck(ctx, uname, claims.Subject)
			if !ok {
				s.Error(writer, status, msg)

				return
			}
		}
	}

	user, err := scanUserPublic(s.Pool.QueryRow(ctx,
		`UPDATE users SET
			username   = COALESCE($1, username),
			bio        = COALESCE($2, bio),
			avatarUrl = COALESCE($3, avatarUrl),
			updatedAt = NOW()
		 WHERE id = $4::uuid
		 RETURNING id::text, username, email, role, avatarUrl, bio, isActive, authProvider, createdAt::text`,
		body.Username, body.Bio, body.AvatarURL, claims.Subject))
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, user)
}

// verifyCurrentPassword checks that candidate matches userID's stored
// password hash. It returns the HTTP status/message to send back when
// verification fails, or ok == true to proceed.
//
//nolint:gocritic // named results here would trip nonamedreturns instead; see doc comment above for the meaning of each value
func (s *State) verifyCurrentPassword(ctx context.Context, userID, candidate string) (int, string, bool) {
	var passwordHash *string

	err := s.Pool.QueryRow(ctx,
		"SELECT password_hash FROM users WHERE id = $1::uuid", userID).Scan(&passwordHash)
	if err != nil || passwordHash == nil {
		return http.StatusBadRequest, "This account uses SSO login and has no local password.", false
	}

	err = bcrypt.CompareHashAndPassword([]byte(*passwordHash), []byte(candidate))
	if err != nil {
		return http.StatusUnauthorized, "Incorrect current password", false
	}

	return http.StatusOK, "", true
}

// ChangePassword godoc
// @Summary   Change the current user's password
// @Tags      Auth
// @Security  BearerAuth
// @Accept    json
// @Produce   json
// @Param     body  object  true  "oldPassword and newPassword"
// @Success   200   {object}  map[string]string
// @Failure   400   {object}  map[string]string
// @Failure   401   {object}  map[string]string
// @Router    /api/auth/password [put].
func (s *State) ChangePassword(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}

	err := decode(request, &body)
	if err != nil {
		s.Error(writer, http.StatusBadRequest, "Invalid JSON")

		return
	}

	if body.OldPassword == "" {
		s.Error(writer, http.StatusBadRequest, "oldPassword required")

		return
	}

	if body.NewPassword == "" {
		s.Error(writer, http.StatusBadRequest, "newPassword required")

		return
	}

	if msg, ok := s.validatePasswordPolicy(request, body.NewPassword); !ok {
		s.Error(writer, http.StatusBadRequest, msg)

		return
	}

	ctx := request.Context()
	claims := s.claims(request)

	status, msg, ok := s.verifyCurrentPassword(ctx, claims.Subject, body.OldPassword)
	if !ok {
		s.Error(writer, status, msg)

		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcryptCost)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Hash error")

		return
	}

	_, err = s.Pool.Exec(ctx,
		"UPDATE users SET password_hash = $1, updatedAt = NOW() WHERE id = $2::uuid",
		string(newHash), claims.Subject)
	if err != nil {
		s.Error(writer, http.StatusInternalServerError, "Database error")

		return
	}

	s.JSON(writer, http.StatusOK, map[string]string{groupsRespKeyMessage: "Password changed successfully"})
}
