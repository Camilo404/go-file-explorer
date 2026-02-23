package model

import "errors"

var (
	// User related errors
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrInvalidCredentials = errors.New("invalid credentials")

	// Token related errors
	ErrTokenNotFound = errors.New("token not found")
	ErrTokenExpired  = errors.New("token expired")

	// File/Directory related errors
	ErrFileNotFound      = errors.New("file not found")
	ErrDirectoryNotFound = errors.New("directory not found")
	ErrPathConflict      = errors.New("path conflict")

	// Permission/Access related errors
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")

	// Job related errors
	ErrJobNotFound = errors.New("job not found")

	// Share related errors
	ErrShareNotFound = errors.New("share not found")
	ErrShareExpired  = errors.New("share expired")

	// Trash related errors
	ErrTrashItemNotFound   = errors.New("trash item not found")
	ErrItemAlreadyRestored = errors.New("item already restored")

	// Generic errors
	ErrInvalidInput = errors.New("invalid input")
)
