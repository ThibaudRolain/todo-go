package user

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"todo-go/internal/task"

	"golang.org/x/crypto/bcrypt"
)

type Store struct {
	mu    sync.Mutex
	path  string
	Users []User `json:"users"`
}

func defaultUsersPath() (string, error) {
	base, err := task.DefaultDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "users.json"), nil
}

func Open(path string) (*Store, error) {
	if path == "" {
		p, err := defaultUsersPath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return s, nil
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func (s *Store) findLocked(username string) *User {
	for i := range s.Users {
		if s.Users[i].Username == username {
			return &s.Users[i]
		}
	}
	return nil
}

func (s *Store) Has(username string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.findLocked(username) != nil
}

func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Users)
}

func (s *Store) Usernames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.Users))
	for i, u := range s.Users {
		out[i] = u.Username
	}
	return out
}

func (s *Store) Register(rawUsername, password string) (string, error) {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.findLocked(username) != nil {
		return "", ErrUserExists
	}
	s.Users = append(s.Users, User{
		Username:     username,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	if err := s.save(); err != nil {
		return "", err
	}
	return username, nil
}

func (s *Store) Authenticate(rawUsername, password string) (string, error) {
	username := NormalizeUsername(rawUsername)
	if username == "" {
		return "", ErrBadCredentials
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.findLocked(username)
	if u == nil {
		return "", ErrBadCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", ErrBadCredentials
	}
	return username, nil
}
