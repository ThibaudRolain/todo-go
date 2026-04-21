package user

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestStore_RegisterAndAuth(t *testing.T) {
	us, _ := Open(filepath.Join(t.TempDir(), "users.json"))
	name, err := us.Register("Alice", "password123")
	if err != nil || name != "alice" {
		t.Fatalf("Register: %v, %q", err, name)
	}
	name, err = us.Authenticate("ALICE", "password123")
	if err != nil || name != "alice" {
		t.Fatalf("Authenticate: %v, %q", err, name)
	}
	if _, err := us.Authenticate("alice", "WRONG"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("want ErrBadCredentials, got %v", err)
	}
	if _, err := us.Authenticate("unknown", "password123"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("want ErrBadCredentials for unknown, got %v", err)
	}
}

func TestStore_RegisterRejectsDuplicates(t *testing.T) {
	us, _ := Open(filepath.Join(t.TempDir(), "users.json"))
	us.Register("alice", "password123")
	if _, err := us.Register("alice", "different"); !errors.Is(err, ErrUserExists) {
		t.Fatalf("want ErrUserExists, got %v", err)
	}
	if _, err := us.Register("ALICE", "different"); !errors.Is(err, ErrUserExists) {
		t.Fatalf("want ErrUserExists normalized, got %v", err)
	}
}

func TestStore_RegisterValidation(t *testing.T) {
	us, _ := Open(filepath.Join(t.TempDir(), "users.json"))
	cases := []struct {
		name           string
		username, pass string
		want           error
	}{
		{"short username", "ab", "password123", ErrInvalidUsername},
		{"invalid char", "a b", "password123", ErrInvalidUsername},
		{"too long", "a1234567890123456789012345678901234", "password123", ErrInvalidUsername},
		{"short password", "alice", "short", ErrPasswordTooShort},
		{"empty password", "alice", "", ErrPasswordTooShort},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := us.Register(c.username, c.pass); !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
}

func TestStore_Persistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	us1, _ := Open(path)
	us1.Register("alice", "password123")

	us2, _ := Open(path)
	if !us2.Has("alice") {
		t.Fatalf("should persist")
	}
	if _, err := us2.Authenticate("alice", "password123"); err != nil {
		t.Fatalf("Authenticate after reopen: %v", err)
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
			t.Fatalf("%q: want %q, got %q", c.in, c.want, got)
		}
	}
}
