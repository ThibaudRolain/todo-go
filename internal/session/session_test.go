package session

import "testing"

func TestManager_IssueGetRevoke(t *testing.T) {
	m := NewManager()
	token := m.Issue("alice")
	if m.Get(token) != "alice" {
		t.Fatalf("Get should return alice")
	}
	m.Revoke(token)
	if m.Get(token) != "" {
		t.Fatalf("Get after Revoke should be empty")
	}
}

func TestManager_UnknownToken(t *testing.T) {
	m := NewManager()
	if m.Get("nope") != "" {
		t.Fatalf("Get for unknown token should be empty")
	}
}
