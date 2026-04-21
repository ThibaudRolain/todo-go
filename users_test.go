package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestUserStore_RegisterAndAuth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	us, err := OpenUsers(path)
	if err != nil {
		t.Fatalf("OpenUsers: %v", err)
	}

	name, err := us.Register("Alice", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if name != "alice" {
		t.Fatalf("Register should normalize; got %q", name)
	}

	name, err = us.Authenticate("ALICE", "password123")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if name != "alice" {
		t.Fatalf("Authenticate name: got %q", name)
	}

	if _, err := us.Authenticate("alice", "WRONG"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("want ErrBadCredentials, got %v", err)
	}

	if _, err := us.Authenticate("unknown", "password123"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("want ErrBadCredentials for unknown user, got %v", err)
	}
}

func TestUserStore_RegisterRejectsDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	us, _ := OpenUsers(path)
	if _, err := us.Register("alice", "password123"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := us.Register("alice", "different"); !errors.Is(err, ErrUserExists) {
		t.Fatalf("want ErrUserExists, got %v", err)
	}
	if _, err := us.Register("ALICE", "different"); !errors.Is(err, ErrUserExists) {
		t.Fatalf("want ErrUserExists (normalized), got %v", err)
	}
}

func TestUserStore_RegisterValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	us, _ := OpenUsers(path)

	cases := []struct {
		name           string
		username, pass string
		want           error
	}{
		{"too short", "ab", "password123", ErrInvalidUsername},
		{"invalid char", "a b", "password123", ErrInvalidUsername},
		{"too long", "a1234567890123456789012345678901234", "password123", ErrInvalidUsername},
		{"short password", "alice", "short", ErrPasswordTooShort},
		{"empty password", "alice", "", ErrPasswordTooShort},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := us.Register(c.username, c.pass)
			if !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
}

func TestUserStore_Persistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")

	us1, _ := OpenUsers(path)
	if _, err := us1.Register("alice", "password123"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	us2, _ := OpenUsers(path)
	if !us2.Has("alice") {
		t.Fatalf("user should persist across reopen")
	}
	if _, err := us2.Authenticate("alice", "password123"); err != nil {
		t.Fatalf("Authenticate after reopen: %v", err)
	}
}

func TestSessionManager_IssueGetRevoke(t *testing.T) {
	sm := NewSessionManager()
	token := sm.Issue("alice")
	if sm.Get(token) != "alice" {
		t.Fatalf("Get should return alice")
	}
	sm.Revoke(token)
	if sm.Get(token) != "" {
		t.Fatalf("Get should return empty after Revoke")
	}
}

func TestSessionManager_UnknownToken(t *testing.T) {
	sm := NewSessionManager()
	if sm.Get("not-a-real-token") != "" {
		t.Fatalf("Get should return empty for unknown token")
	}
}

func TestNormalizeUsername(t *testing.T) {
	cases := []struct{ in, want string }{
		{"alice", "alice"},
		{"  Alice  ", "alice"},
		{"ALICE_99", "alice_99"},
		{"with-dash", "with-dash"},
		{"has space", ""},
		{"", ""},
		{"ab", ""},
		{"with!bang", ""},
	}
	for _, c := range cases {
		if got := NormalizeUsername(c.in); got != c.want {
			t.Fatalf("NormalizeUsername(%q): want %q, got %q", c.in, c.want, got)
		}
	}
}
