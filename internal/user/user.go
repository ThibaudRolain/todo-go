package user

import (
	"errors"
	"regexp"
	"strings"
)

type User struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	CreatedAt    string `json:"created_at"`
}

var (
	ErrUserExists       = errors.New("user already exists")
	ErrBadCredentials   = errors.New("invalid username or password")
	ErrInvalidUsername  = errors.New("username must be 3–32 chars of [a-z0-9_-]")
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
)

var usernameRe = regexp.MustCompile(`^[a-z0-9_-]{3,32}$`)

func NormalizeUsername(raw string) string {
	u := strings.ToLower(strings.TrimSpace(raw))
	if !usernameRe.MatchString(u) {
		return ""
	}
	return u
}
