package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"go-file-explorer/internal/model"
	"go-file-explorer/pkg/apierror"
)

type AuthService struct {
	usersFile       string
	jwtSecret       []byte
	accessTTL       time.Duration
	refreshTTL      time.Duration
	mu              sync.RWMutex
	usersByUsername map[string]model.User
	usersByID       map[string]model.User
	refreshTokens   map[string]string
}

func NewAuthService(usersFile string, jwtSecret string, accessTTL time.Duration, refreshTTL time.Duration) (*AuthService, error) {
	service := &AuthService{
		usersFile:       usersFile,
		jwtSecret:       []byte(jwtSecret),
		accessTTL:       accessTTL,
		refreshTTL:      refreshTTL,
		usersByUsername: map[string]model.User{},
		usersByID:       map[string]model.User{},
		refreshTokens:   map[string]string{},
	}

	if err := service.loadUsers(); err != nil {
		return nil, err
	}

	return service, nil
}

func (s *AuthService) Login(username string, password string) (model.TokenPair, error) {
	s.mu.RLock()
	user, exists := s.usersByUsername[strings.ToLower(strings.TrimSpace(username))]
	s.mu.RUnlock()
	if !exists {
		return model.TokenPair{}, apierror.New("UNAUTHORIZED", "invalid credentials", "", http.StatusUnauthorized)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return model.TokenPair{}, apierror.New("UNAUTHORIZED", "invalid credentials", "", http.StatusUnauthorized)
	}

	return s.issueTokenPair(user)
}

func (s *AuthService) Register(username string, password string, role string) (model.AuthUser, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	role = strings.ToLower(strings.TrimSpace(role))

	if username == "" || password == "" {
		return model.AuthUser{}, apierror.New("BAD_REQUEST", "username and password are required", "", http.StatusBadRequest)
	}
	if role == "" {
		role = "viewer"
	}
	if role != "admin" && role != "editor" && role != "viewer" {
		return model.AuthUser{}, apierror.New("BAD_REQUEST", "invalid role", role, http.StatusBadRequest)
	}

	key := strings.ToLower(username)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.usersByUsername[key]; exists {
		return model.AuthUser{}, apierror.New("ALREADY_EXISTS", "username already exists", username, http.StatusConflict)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return model.AuthUser{}, err
	}

	now := time.Now().UTC()
	user := model.User{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	s.usersByUsername[key] = user
	s.usersByID[user.ID] = user

	if err := s.saveUsersLocked(); err != nil {
		return model.AuthUser{}, err
	}

	return model.AuthUser{ID: user.ID, Username: user.Username, Role: user.Role}, nil
}

func (s *AuthService) Refresh(refreshToken string) (model.TokenPair, error) {
	claims, err := s.ValidateToken(refreshToken, "refresh")
	if err != nil {
		return model.TokenPair{}, err
	}

	s.mu.Lock()
	ownerID, exists := s.refreshTokens[refreshToken]
	if !exists || ownerID != claims.UserID {
		s.mu.Unlock()
		return model.TokenPair{}, apierror.New("UNAUTHORIZED", "refresh token is invalid", "", http.StatusUnauthorized)
	}
	delete(s.refreshTokens, refreshToken)
	user, userExists := s.usersByID[claims.UserID]
	s.mu.Unlock()

	if !userExists {
		return model.TokenPair{}, apierror.New("UNAUTHORIZED", "user not found", "", http.StatusUnauthorized)
	}

	return s.issueTokenPair(user)
}

func (s *AuthService) Logout(refreshToken string) {
	s.mu.Lock()
	delete(s.refreshTokens, refreshToken)
	s.mu.Unlock()
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
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.usersByID[userID]
	if !exists {
		return model.AuthUser{}, apierror.New("NOT_FOUND", "user not found", userID, http.StatusNotFound)
	}

	return model.AuthUser{ID: user.ID, Username: user.Username, Role: user.Role}, nil
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

	s.mu.Lock()
	s.refreshTokens[refreshToken] = user.ID
	s.mu.Unlock()

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

func (s *AuthService) loadUsers() error {
	if strings.TrimSpace(s.usersFile) == "" {
		return errors.New("users file path is required")
	}

	if err := os.MkdirAll(filepath.Dir(s.usersFile), 0o755); err != nil {
		return err
	}

	if _, err := os.Stat(s.usersFile); os.IsNotExist(err) {
		if err := s.seedDefaultAdmin(); err != nil {
			return err
		}
	}

	data, err := os.ReadFile(s.usersFile)
	if err != nil {
		return err
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		if err := s.seedDefaultAdmin(); err != nil {
			return err
		}
		data, err = os.ReadFile(s.usersFile)
		if err != nil {
			return err
		}
	}

	var users []model.User
	if err := json.Unmarshal(data, &users); err != nil {
		return err
	}
	if len(users) == 0 {
		if err := s.seedDefaultAdmin(); err != nil {
			return err
		}
		return s.loadUsers()
	}

	usersByUsername := map[string]model.User{}
	usersByID := map[string]model.User{}
	for _, user := range users {
		usersByUsername[strings.ToLower(user.Username)] = user
		usersByID[user.ID] = user
	}

	s.mu.Lock()
	s.usersByUsername = usersByUsername
	s.usersByID = usersByID
	s.mu.Unlock()

	return nil
}

func (s *AuthService) seedDefaultAdmin() error {
	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), 12)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	defaultAdmin := []model.User{{
		ID:           uuid.NewString(),
		Username:     "admin",
		PasswordHash: string(hash),
		Role:         "admin",
		CreatedAt:    now,
		UpdatedAt:    now,
	}}

	data, err := json.MarshalIndent(defaultAdmin, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.usersFile, data, 0o600)
}

func (s *AuthService) saveUsersLocked() error {
	users := make([]model.User, 0, len(s.usersByID))
	for _, user := range s.usersByID {
		users = append(users, user)
	}

	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.usersFile, data, 0o600)
}
