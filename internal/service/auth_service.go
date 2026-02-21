package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/repository"
	"go-file-explorer/pkg/apierror"
)

const (
	// MaxFailedAttempts is the number of consecutive failed logins before lockout.
	MaxFailedAttempts = 5
	// LockoutDuration is how long an account stays locked after exceeding MaxFailedAttempts.
	LockoutDuration = 15 * time.Minute
	// MinPasswordLength is the minimum acceptable password length.
	MinPasswordLength = 8
	// MinJWTSecretLength is the minimum acceptable JWT secret length.
	MinJWTSecretLength = 32
)

type AuthService struct {
	jwtSecret  []byte
	accessTTL  time.Duration
	refreshTTL time.Duration

	userRepo  *repository.UserRepository
	tokenRepo *repository.TokenRepository
}

func NewAuthService(jwtSecret string, accessTTL time.Duration, refreshTTL time.Duration, userRepo *repository.UserRepository, tokenRepo *repository.TokenRepository) (*AuthService, error) {
	if len(jwtSecret) < MinJWTSecretLength {
		return nil, fmt.Errorf("JWT_SECRET must be at least %d characters long (got %d); generate one with: openssl rand -base64 48", MinJWTSecretLength, len(jwtSecret))
	}

	service := &AuthService{
		jwtSecret:  []byte(jwtSecret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		userRepo:   userRepo,
		tokenRepo:  tokenRepo,
	}

	ctx := context.Background()
	count, err := userRepo.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		if err := service.seedDefaultAdmin(ctx); err != nil {
			return nil, err
		}
	}

	return service, nil
}

func (s *AuthService) Login(username string, password string) (model.TokenPair, error) {
	ctx := context.Background()
	user, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		return model.TokenPair{}, apierror.New("UNAUTHORIZED", "invalid credentials", "", http.StatusUnauthorized)
	}

	// ── Account lockout check ────────────────────────────────────
	if user.LockedUntil != nil && time.Now().UTC().Before(*user.LockedUntil) {
		remaining := time.Until(*user.LockedUntil).Round(time.Second)
		return model.TokenPair{}, apierror.New("ACCOUNT_LOCKED",
			fmt.Sprintf("account is locked, try again in %s", remaining), "", http.StatusTooManyRequests)
	}

	// ── Password verification ────────────────────────────────────
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		// Increment failed attempts
		_ = s.userRepo.IncrementFailedAttempts(ctx, user.ID)
		newCount := user.FailedLoginAttempts + 1

		if newCount >= MaxFailedAttempts {
			lockUntil := time.Now().UTC().Add(LockoutDuration)
			_ = s.userRepo.LockAccount(ctx, user.ID, lockUntil)
			slog.Warn("account locked due to too many failed attempts",
				"username", username, "attempts", newCount, "locked_until", lockUntil)
			return model.TokenPair{}, apierror.New("ACCOUNT_LOCKED",
				fmt.Sprintf("too many failed attempts, account locked for %s", LockoutDuration), "", http.StatusTooManyRequests)
		}

		return model.TokenPair{}, apierror.New("UNAUTHORIZED", "invalid credentials", "", http.StatusUnauthorized)
	}

	// ── Successful login: reset failed attempts ──────────────────
	if user.FailedLoginAttempts > 0 {
		_ = s.userRepo.ResetFailedAttempts(ctx, user.ID)
	}

	pair, err := s.issueTokenPair(user)
	if err != nil {
		return model.TokenPair{}, err
	}

	// Signal the client that a password change is required.
	if user.ForcePasswordChange {
		pair.ForcePasswordChange = true
	}

	return pair, nil
}

func (s *AuthService) Register(username string, password string, role string) (model.AuthUser, error) {
	ctx := context.Background()
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	role = strings.ToLower(strings.TrimSpace(role))

	if username == "" || password == "" {
		return model.AuthUser{}, apierror.New("BAD_REQUEST", "username and password are required", "", http.StatusBadRequest)
	}
	if err := validatePasswordStrength(password); err != nil {
		return model.AuthUser{}, err
	}
	if role == "" {
		role = "viewer"
	}
	if role != "admin" && role != "editor" && role != "viewer" {
		return model.AuthUser{}, apierror.New("BAD_REQUEST", "invalid role", role, http.StatusBadRequest)
	}

	exists, err := s.userRepo.ExistsByUsername(ctx, username)
	if err != nil {
		return model.AuthUser{}, err
	}
	if exists {
		return model.AuthUser{}, apierror.New("ALREADY_EXISTS", "username already exists", username, http.StatusConflict)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return model.AuthUser{}, err
	}

	now := time.Now().UTC()
	user := model.User{
		ID:                  uuid.NewString(),
		Username:            username,
		PasswordHash:        string(hash),
		Role:                role,
		ForcePasswordChange: false,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return model.AuthUser{}, err
	}

	return model.AuthUser{ID: user.ID, Username: user.Username, Role: user.Role}, nil
}

func (s *AuthService) Refresh(refreshToken string) (model.TokenPair, error) {
	ctx := context.Background()
	claims, err := s.ValidateToken(refreshToken, "refresh")
	if err != nil {
		return model.TokenPair{}, err
	}

	ownerID, err := s.tokenRepo.Validate(ctx, refreshToken)
	if err != nil || ownerID != claims.UserID {
		return model.TokenPair{}, apierror.New("UNAUTHORIZED", "refresh token is invalid", "", http.StatusUnauthorized)
	}

	_ = s.tokenRepo.Revoke(ctx, refreshToken)

	user, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		return model.TokenPair{}, apierror.New("UNAUTHORIZED", "user not found", "", http.StatusUnauthorized)
	}

	pair, err := s.issueTokenPair(user)
	if err != nil {
		return model.TokenPair{}, err
	}

	if user.ForcePasswordChange {
		pair.ForcePasswordChange = true
	}

	return pair, nil
}

func (s *AuthService) Logout(refreshToken string) {
	ctx := context.Background()
	_ = s.tokenRepo.Revoke(ctx, refreshToken)
}

func (s *AuthService) ValidateToken(tokenString string, expectedType string) (*model.AuthClaims, error) {
	parsed, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, apierror.New("UNAUTHORIZED", "invalid token signing method", "", http.StatusUnauthorized)
		}
		return s.jwtSecret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, apierror.New("UNAUTHORIZED", "invalid token", "", http.StatusUnauthorized)
	}

	claimsMap, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, apierror.New("UNAUTHORIZED", "invalid token claims", "", http.StatusUnauthorized)
	}

	typ, _ := claimsMap["typ"].(string)
	if expectedType != "" && typ != expectedType {
		return nil, apierror.New("UNAUTHORIZED", "invalid token type", "", http.StatusUnauthorized)
	}

	claims := &model.AuthClaims{Type: typ}
	claims.UserID, _ = claimsMap["sub"].(string)
	claims.Username, _ = claimsMap["username"].(string)
	claims.Role, _ = claimsMap["role"].(string)
	claims.TokenID, _ = claimsMap["jti"].(string)

	if claims.UserID == "" {
		return nil, apierror.New("UNAUTHORIZED", "invalid token subject", "", http.StatusUnauthorized)
	}

	return claims, nil
}

func (s *AuthService) GetUserByID(userID string) (model.AuthUser, error) {
	ctx := context.Background()
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return model.AuthUser{}, err
	}
	return model.AuthUser{ID: user.ID, Username: user.Username, Role: user.Role, ForcePasswordChange: user.ForcePasswordChange}, nil
}

func (s *AuthService) ListUsers() []model.AuthUser {
	ctx := context.Background()
	users, err := s.userRepo.List(ctx)
	if err != nil {
		return []model.AuthUser{}
	}
	return users
}

func (s *AuthService) UpdateUser(userID string, role string) (model.AuthUser, error) {
	ctx := context.Background()
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return model.AuthUser{}, apierror.New("BAD_REQUEST", "role is required", "", http.StatusBadRequest)
	}
	if role != "admin" && role != "editor" && role != "viewer" {
		return model.AuthUser{}, apierror.New("BAD_REQUEST", "invalid role", role, http.StatusBadRequest)
	}

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return model.AuthUser{}, err
	}

	user.Role = role
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(ctx, user); err != nil {
		return model.AuthUser{}, err
	}

	return model.AuthUser{ID: user.ID, Username: user.Username, Role: user.Role}, nil
}

func (s *AuthService) DeleteUser(userID string, callerID string) error {
	if userID == callerID {
		return apierror.New("BAD_REQUEST", "cannot delete your own account", "", http.StatusBadRequest)
	}

	ctx := context.Background()
	return s.userRepo.Delete(ctx, userID)
}

func (s *AuthService) ChangePassword(userID string, currentPassword string, newPassword string) error {
	if strings.TrimSpace(newPassword) == "" {
		return apierror.New("BAD_REQUEST", "new_password is required", "", http.StatusBadRequest)
	}

	if err := validatePasswordStrength(newPassword); err != nil {
		return err
	}

	ctx := context.Background()
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return apierror.New("UNAUTHORIZED", "current password is incorrect", "", http.StatusUnauthorized)
	}

	// Prevent reusing the same password.
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(newPassword)) == nil {
		return apierror.New("BAD_REQUEST", "new password must be different from the current password", "", http.StatusBadRequest)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		return err
	}

	if err := s.userRepo.UpdatePassword(ctx, userID, string(hash)); err != nil {
		return err
	}

	// Invalidate all existing refresh tokens for this user so that
	// any previously issued sessions are revoked after a password change.
	_ = s.tokenRepo.RevokeAllForUser(ctx, userID)

	return nil
}

func (s *AuthService) issueTokenPair(user model.User) (model.TokenPair, error) {
	now := time.Now().UTC()
	accessJTI := uuid.NewString()
	refreshJTI := uuid.NewString()

	accessToken, err := s.signToken(jwt.MapClaims{
		"sub":      user.ID,
		"username": user.Username,
		"role":     user.Role,
		"typ":      "access",
		"jti":      accessJTI,
		"iat":      now.Unix(),
		"exp":      now.Add(s.accessTTL).Unix(),
	})
	if err != nil {
		return model.TokenPair{}, err
	}

	refreshToken, err := s.signToken(jwt.MapClaims{
		"sub":      user.ID,
		"username": user.Username,
		"role":     user.Role,
		"typ":      "refresh",
		"jti":      refreshJTI,
		"iat":      now.Unix(),
		"exp":      now.Add(s.refreshTTL).Unix(),
	})
	if err != nil {
		return model.TokenPair{}, err
	}

	ctx := context.Background()
	expiresAt := now.Add(s.refreshTTL)
	if storeErr := s.tokenRepo.Store(ctx, refreshToken, user.ID, expiresAt); storeErr != nil {
		return model.TokenPair{}, storeErr
	}

	return model.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.accessTTL.Seconds()),
		User:         model.AuthUser{ID: user.ID, Username: user.Username, Role: user.Role},
	}, nil
}

func (s *AuthService) signToken(claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *AuthService) seedDefaultAdmin(ctx context.Context) error {
	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), 12)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	user := model.User{
		ID:                  uuid.NewString(),
		Username:            "admin",
		PasswordHash:        string(hash),
		Role:                "admin",
		ForcePasswordChange: true, // Force password change on first login
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	slog.Warn("seeding default admin user — change the password immediately after first login",
		"username", user.Username, "force_password_change", true)

	return s.userRepo.Create(ctx, user)
}

// validatePasswordStrength enforces minimum password complexity requirements.
// Rules:
//   - At least MinPasswordLength characters
//   - At least one uppercase letter
//   - At least one lowercase letter
//   - At least one digit
//   - At least one special character
func validatePasswordStrength(password string) error {
	if len(password) < MinPasswordLength {
		return apierror.New("WEAK_PASSWORD",
			fmt.Sprintf("password must be at least %d characters long", MinPasswordLength), "", http.StatusBadRequest)
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSpecial = true
		}
	}

	var missing []string
	if !hasUpper {
		missing = append(missing, "uppercase letter")
	}
	if !hasLower {
		missing = append(missing, "lowercase letter")
	}
	if !hasDigit {
		missing = append(missing, "digit")
	}
	if !hasSpecial {
		missing = append(missing, "special character")
	}

	if len(missing) > 0 {
		return apierror.New("WEAK_PASSWORD",
			fmt.Sprintf("password must contain at least one: %s", strings.Join(missing, ", ")), "", http.StatusBadRequest)
	}

	return nil
}
