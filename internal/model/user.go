package model

import "time"

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AuthClaims struct {
	UserID   string `json:"sub"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Type     string `json:"typ"`
	TokenID  string `json:"jti"`
}

type AuthUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type TokenPair struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	TokenType    string   `json:"token_type"`
	ExpiresIn    int64    `json:"expires_in"`
	User         AuthUser `json:"user"`
}
