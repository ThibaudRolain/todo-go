package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

const CookieName = "tg_session"

type entry struct {
	username string
	created  time.Time
}

type Manager struct {
	mu       sync.Mutex
	sessions map[string]entry
}

func NewManager() *Manager {
	return &Manager{sessions: make(map[string]entry)}
}

func (m *Manager) Issue(username string) string {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	token := base64.RawURLEncoding.EncodeToString(buf[:])
	m.mu.Lock()
	m.sessions[token] = entry{username: username, created: time.Now()}
	m.mu.Unlock()
	return token
}

func (m *Manager) Get(token string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.sessions[token]
	if !ok {
		return ""
	}
	return e.username
}

func (m *Manager) Revoke(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
}

func (m *Manager) UserFromRequest(r *http.Request) string {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return ""
	}
	return m.Get(c.Value)
}

type ctxKey int

const userCtxKey ctxKey = 1

func UsernameFromRequest(r *http.Request) string {
	v := r.Context().Value(userCtxKey)
	if v == nil {
		return ""
	}
	u, _ := v.(string)
	return u
}

func SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func (m *Manager) RequireAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := m.UserFromRequest(r)
		if user == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, user)
		h.ServeHTTP(w, r.WithContext(ctx))
	}
}
