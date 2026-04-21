package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
)

//go:embed web
var webFS embed.FS

type serverDeps struct {
	Users    *UserStore
	Stores   *StoreManager
	Sessions *SessionManager
}

func runServer(deps serverDeps, addr string) error {
	mux := http.NewServeMux()

	// Auth API (public)
	mux.HandleFunc("/api/register", apiRegisterHandler(deps))
	mux.HandleFunc("/api/login", apiLoginHandler(deps))
	mux.HandleFunc("/api/logout", apiLogoutHandler(deps))
	mux.HandleFunc("/api/me", deps.Sessions.requireAuth(apiMeHandler()))

	// Task API (protected)
	mux.HandleFunc("/api/tasks", deps.Sessions.requireAuth(apiTasksHandler(deps.Stores)))
	mux.HandleFunc("/api/tasks/", deps.Sessions.requireAuth(apiTaskByIDHandler(deps.Stores)))
	mux.HandleFunc("/api/reorder", deps.Sessions.requireAuth(apiReorderHandler(deps.Stores)))
	mux.HandleFunc("/api/labels", deps.Sessions.requireAuth(apiLabelsHandler(deps.Stores)))

	// Static web (gates index.html behind auth; login/register are public)
	webRoot, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}
	staticFS := http.FileServer(http.FS(webRoot))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Public pages
		switch r.URL.Path {
		case "/login", "/register":
			serveStaticPage(w, r, webRoot, r.URL.Path+".html")
			return
		}
		// Any other path: require auth
		if deps.Sessions.userFromRequest(r) == "" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		staticFS.ServeHTTP(w, r)
	})

	fmt.Printf("todo-go serving on http://%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func serveStaticPage(w http.ResponseWriter, r *http.Request, webRoot fs.FS, name string) {
	name = strings.TrimPrefix(name, "/")
	data, err := fs.ReadFile(webRoot, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// --- Auth routes -----------------------------------------------------------

func apiRegisterHandler(deps serverDeps) http.HandlerFunc {
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
			case errors.Is(err, ErrUserExists):
				http.Error(w, err.Error(), http.StatusConflict)
			case errors.Is(err, ErrInvalidUsername), errors.Is(err, ErrPasswordTooShort):
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
			return
		}

		// On first-ever user, adopt any legacy tasks.json.
		if firstUser {
			if _, migErr := migrateLegacyTasks(username); migErr != nil {
				fmt.Printf("warning: legacy task migration failed for %s: %v\n", username, migErr)
			}
		}

		token := deps.Sessions.Issue(username)
		setSessionCookie(w, token)
		writeJSON(w, http.StatusCreated, map[string]string{"username": username})
	}
}

func apiLoginHandler(deps serverDeps) http.HandlerFunc {
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
		setSessionCookie(w, token)
		writeJSON(w, http.StatusOK, map[string]string{"username": username})
	}
}

func apiLogoutHandler(deps serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if c, err := r.Cookie(sessionCookieName); err == nil {
			deps.Sessions.Revoke(c.Value)
		}
		clearSessionCookie(w)
		w.WriteHeader(http.StatusNoContent)
	}
}

func apiMeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"username": usernameFromRequest(r)})
	}
}

// --- Task routes (per-user) ------------------------------------------------

func userStore(w http.ResponseWriter, r *http.Request, mgr *StoreManager) *Store {
	user := usernameFromRequest(r)
	s, err := mgr.ForUser(user)
	if err != nil {
		http.Error(w, "failed to open store", http.StatusInternalServerError)
		return nil
	}
	return s
}

func apiTasksHandler(mgr *StoreManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store := userStore(w, r, mgr)
		if store == nil {
			return
		}
		switch r.Method {
		case http.MethodGet:
			tasks := store.List()
			switch r.URL.Query().Get("status") {
			case "pending":
				tasks = filterByDone(tasks, false)
			case "done":
				tasks = filterByDone(tasks, true)
			case "", "all":
			default:
				http.Error(w, "invalid status; want one of: pending, done, all", http.StatusBadRequest)
				return
			}
			if label := r.URL.Query().Get("label"); label != "" {
				tasks = filterByLabel(tasks, label)
			}
			sortParam := r.URL.Query().Get("sort")
			if sortParam == "" {
				sortParam = string(SortByDue)
			}
			mode := SortMode(sortParam)
			if mode != SortManual && mode != SortByDue {
				http.Error(w, "invalid sort; want one of: due, manual", http.StatusBadRequest)
				return
			}
			SortTasks(tasks, mode)
			writeJSON(w, http.StatusOK, tasks)

		case http.MethodPost:
			var body struct {
				Title   string   `json:"title"`
				DueDate string   `json:"due_date"`
				Labels  []string `json:"labels"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			title := strings.TrimSpace(body.Title)
			if title == "" {
				http.Error(w, "title required", http.StatusBadRequest)
				return
			}
			t, err := store.Add(NewTask{Title: title, DueDate: body.DueDate, Labels: body.Labels})
			if err != nil {
				writeStoreErr(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, t)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func apiTaskByIDHandler(mgr *StoreManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store := userStore(w, r, mgr)
		if store == nil {
			return
		}
		idStr := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
		idStr = strings.TrimSuffix(idStr, "/")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodPatch:
			var body struct {
				Done    *bool     `json:"done"`
				Title   *string   `json:"title"`
				DueDate *string   `json:"due_date"`
				Labels  *[]string `json:"labels"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			if body.Done == nil && body.Title == nil && body.DueDate == nil && body.Labels == nil {
				http.Error(w, "no fields to update", http.StatusBadRequest)
				return
			}
			var updated Task
			if body.Title != nil {
				title := strings.TrimSpace(*body.Title)
				if title == "" {
					http.Error(w, "title must not be empty", http.StatusBadRequest)
					return
				}
				updated, err = store.SetTitle(id, title)
				if err != nil {
					writeStoreErr(w, err)
					return
				}
			}
			if body.DueDate != nil {
				updated, err = store.SetDue(id, strings.TrimSpace(*body.DueDate))
				if err != nil {
					writeStoreErr(w, err)
					return
				}
			}
			if body.Labels != nil {
				updated, err = store.SetLabels(id, *body.Labels)
				if err != nil {
					writeStoreErr(w, err)
					return
				}
			}
			if body.Done != nil {
				updated, err = store.SetDone(id, *body.Done)
				if err != nil {
					writeStoreErr(w, err)
					return
				}
			}
			writeJSON(w, http.StatusOK, updated)

		case http.MethodDelete:
			if err := store.Remove(id); err != nil {
				writeStoreErr(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func apiReorderHandler(mgr *StoreManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store := userStore(w, r, mgr)
		if store == nil {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			IDs []int `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if err := store.Reorder(body.IDs); err != nil {
			switch {
			case errors.Is(err, ErrReorderLength), errors.Is(err, ErrReorderUnknown):
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		writeJSON(w, http.StatusOK, store.List())
	}
}

func apiLabelsHandler(mgr *StoreManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store := userStore(w, r, mgr)
		if store == nil {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, store.Labels())
	}
}

func writeStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrEmptyTitle), errors.Is(err, ErrBadDueDate), errors.Is(err, ErrBadLabel):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
