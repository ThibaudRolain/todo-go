package server

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"todo-go/internal/session"
	"todo-go/internal/task"
	"todo-go/internal/user"
)

//go:embed web
var webFS embed.FS

type Deps struct {
	Users    *user.Store
	Stores   *task.Manager
	Sessions *session.Manager
}

func Run(deps Deps, addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/register", registerHandler(deps))
	mux.HandleFunc("/api/login", loginHandler(deps))
	mux.HandleFunc("/api/logout", logoutHandler(deps))
	mux.HandleFunc("/api/me", deps.Sessions.RequireAuth(meHandler()))

	mux.HandleFunc("/api/tasks", deps.Sessions.RequireAuth(tasksHandler(deps)))
	mux.HandleFunc("/api/tasks/", deps.Sessions.RequireAuth(taskByIDHandler(deps.Stores)))
	mux.HandleFunc("/api/reorder", deps.Sessions.RequireAuth(reorderHandler(deps.Stores)))
	mux.HandleFunc("/api/labels", deps.Sessions.RequireAuth(labelsHandler(deps)))
	mux.HandleFunc("/api/public-labels", deps.Sessions.RequireAuth(publicLabelsHandler(deps.Stores)))

	webRoot, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}
	staticFS := http.FileServer(http.FS(webRoot))

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		serveStaticPage(w, r, webRoot, "login.html")
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		serveStaticPage(w, r, webRoot, "register.html")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			if deps.Sessions.UserFromRequest(r) == "" {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, task.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, task.ErrEmptyTitle),
		errors.Is(err, task.ErrBadDueDate),
		errors.Is(err, task.ErrBadLabel):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
