package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	CreatedAt    string `json:"created_at"` // RFC3339
}

type UserStore struct {
	mu    sync.Mutex
	path  string
	Users []User `json:"users"`
}

var (
	ErrUserExists         = errors.New("user already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrBadCredentials     = errors.New("invalid username or password")
	ErrInvalidUsername    = errors.New("username must be 3–32 chars of [a-z0-9_-]")
	ErrPasswordTooShort   = errors.New("password must be at least 8 characters")
)

var usernameRe = regexp.MustCompile(`^[a-z0-9_-]{3,32}$`)

// NormalizeUsername lowercases and trims. Returns "" for invalid inputs.
func NormalizeUsername(raw string) string {
	u := strings.ToLower(strings.TrimSpace(raw))
	if !usernameRe.MatchString(u) {
		return ""
	}
	return u
}

func usersFilePath() (string, error) {
	base, err := defaultDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "users.json"), nil
}

func OpenUsers(path string) (*UserStore, error) {
	if path == "" {
		p, err := usersFilePath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	us := &UserStore{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return us, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, us); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return us, nil
}

func (us *UserStore) save() error {
	if err := os.MkdirAll(filepath.Dir(us.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(us, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(us.path, data, 0o600) // 0600 — password hashes
}

// findLocked assumes the mutex is already held.
func (us *UserStore) findLocked(username string) (*User, int) {
	for i := range us.Users {
		if us.Users[i].Username == username {
			return &us.Users[i], i
		}
	}
	return nil, -1
}

func (us *UserStore) Has(username string) bool {
	us.mu.Lock()
	defer us.mu.Unlock()
	u, _ := us.findLocked(username)
	return u != nil
}

func (us *UserStore) Count() int {
	us.mu.Lock()
	defer us.mu.Unlock()
	return len(us.Users)
}

// Usernames returns all registered usernames (snapshot).
func (us *UserStore) Usernames() []string {
	us.mu.Lock()
	defer us.mu.Unlock()
	out := make([]string, len(us.Users))
	for i, u := range us.Users {
		out[i] = u.Username
	}
	return out
}

// Register creates a new user. Returns the normalized username.
func (us *UserStore) Register(rawUsername, password string) (string, error) {
	username := NormalizeUsername(rawUsername)
	if username == "" {
		return "", ErrInvalidUsername
	}
	if len(password) < 8 {
		return "", ErrPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	us.mu.Lock()
	defer us.mu.Unlock()
	if existing, _ := us.findLocked(username); existing != nil {
		return "", ErrUserExists
	}
	us.Users = append(us.Users, User{
		Username:     username,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	if err := us.save(); err != nil {
		return "", err
	}
	return username, nil
}

// Authenticate returns the normalized username on success.
func (us *UserStore) Authenticate(rawUsername, password string) (string, error) {
	username := NormalizeUsername(rawUsername)
	if username == "" {
		return "", ErrBadCredentials
	}

	us.mu.Lock()
	defer us.mu.Unlock()
	u, _ := us.findLocked(username)
	if u == nil {
		return "", ErrBadCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", ErrBadCredentials
	}
	return username, nil
}
