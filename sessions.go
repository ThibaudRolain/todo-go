package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

type session struct {
	username string
	created  time.Time
}

type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]session
}

func NewSessionManager() *SessionManager {
	return &SessionManager{sessions: make(map[string]session)}
}

const sessionCookieName = "tg_session"

// Issue creates a new session for username and returns its token.
func (sm *SessionManager) Issue(username string) string {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand never fails in practice; panic would be simplest
		panic(err)
	}
	token := base64.RawURLEncoding.EncodeToString(buf[:])
	sm.mu.Lock()
	sm.sessions[token] = session{username: username, created: time.Now()}
	sm.mu.Unlock()
	return token
}

// Get returns the username for a token, or "" if invalid.
func (sm *SessionManager) Get(token string) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.sessions[token]
	if !ok {
		return ""
	}
	return s.username
}

func (sm *SessionManager) Revoke(token string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, token)
}

// ctxKey for storing the username in request context.
type ctxKey int

const userCtxKey ctxKey = 1

func usernameFromRequest(r *http.Request) string {
	v := r.Context().Value(userCtxKey)
	if v == nil {
		return ""
	}
	u, _ := v.(string)
	return u
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		// Secure: true,  // TODO when serving over HTTPS
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// requireAuth wraps a handler; 401 when no valid session.
// On success, the username is attached to the request context.
func (sm *SessionManager) requireAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := sm.userFromRequest(r)
		if user == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, user)
		h.ServeHTTP(w, r.WithContext(ctx))
	}
}

func (sm *SessionManager) userFromRequest(r *http.Request) string {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return sm.Get(c.Value)
}
