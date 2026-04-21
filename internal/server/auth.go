package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"todo-go/internal/session"
	"todo-go/internal/task"
	"todo-go/internal/user"
)

func registerHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		firstUser := deps.Users.Count() == 0
		username, err := deps.Users.Register(body.Username, body.Password)
		if err != nil {
			switch {
			case errors.Is(err, user.ErrUserExists):
				http.Error(w, err.Error(), http.StatusConflict)
			case errors.Is(err, user.ErrInvalidUsername), errors.Is(err, user.ErrPasswordTooShort):
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
			return
		}
		if firstUser {
			if _, migErr := task.MigrateLegacy(username); migErr != nil {
				fmt.Printf("warning: legacy migration failed for %s: %v\n", username, migErr)
			}
		}
		token := deps.Sessions.Issue(username)
		session.SetCookie(w, token)
		writeJSON(w, http.StatusCreated, map[string]string{"username": username})
	}
}

func loginHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		username, err := deps.Users.Authenticate(body.Username, body.Password)
		if err != nil {
			http.Error(w, "invalid username or password", http.StatusUnauthorized)
			return
		}
		token := deps.Sessions.Issue(username)
		session.SetCookie(w, token)
		writeJSON(w, http.StatusOK, map[string]string{"username": username})
	}
}

func logoutHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if c, err := r.Cookie(session.CookieName); err == nil {
			deps.Sessions.Revoke(c.Value)
		}
		session.ClearCookie(w)
		w.WriteHeader(http.StatusNoContent)
	}
}

func meHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"username": session.UsernameFromRequest(r)})
	}
}
