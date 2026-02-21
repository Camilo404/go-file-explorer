package service

import (
"context"
"net/http"
"strings"
"time"

"github.com/golang-jwt/jwt/v5"
"github.com/google/uuid"
"golang.org/x/crypto/bcrypt"

"go-file-explorer/internal/model"
"go-file-explorer/internal/repository"
"go-file-explorer/pkg/apierror"
)

type AuthService struct {
jwtSecret  []byte
accessTTL  time.Duration
refreshTTL time.Duration

userRepo  *repository.UserRepository
tokenRepo *repository.TokenRepository
}

func NewAuthService(jwtSecret string, accessTTL time.Duration, refreshTTL time.Duration, userRepo *repository.UserRepository, tokenRepo *repository.TokenRepository) (*AuthService, error) {
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

if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
return model.TokenPair{}, apierror.New("UNAUTHORIZED", "invalid credentials", "", http.StatusUnauthorized)
}

return s.issueTokenPair(user)
}

func (s *AuthService) Register(username string, password string, role string) (model.AuthUser, error) {
ctx := context.Background()
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
ID:           uuid.NewString(),
Username:     username,
PasswordHash: string(hash),
Role:         role,
CreatedAt:    now,
UpdatedAt:    now,
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

return s.issueTokenPair(user)
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
return model.AuthUser{ID: user.ID, Username: user.Username, Role: user.Role}, nil
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

ctx := context.Background()
user, err := s.userRepo.FindByID(ctx, userID)
if err != nil {
return err
}

if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
return apierror.New("UNAUTHORIZED", "current password is incorrect", "", http.StatusUnauthorized)
}

hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
if err != nil {
return err
}

return s.userRepo.UpdatePassword(ctx, userID, string(hash))
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
ID:           uuid.NewString(),
Username:     "admin",
PasswordHash: string(hash),
Role:         "admin",
CreatedAt:    now,
UpdatedAt:    now,
}

return s.userRepo.Create(ctx, user)
}
