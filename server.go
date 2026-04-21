package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"sort"
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

// TaskView adds owner context when rendering a task from the API.
type TaskView struct {
	Task
	Owner  string `json:"owner"`
	Public bool   `json:"public"` // true if any of its labels is in owner's public set
}

func runServer(deps serverDeps, addr string) error {
	mux := http.NewServeMux()

	// Auth API (public)
	mux.HandleFunc("/api/register", apiRegisterHandler(deps))
	mux.HandleFunc("/api/login", apiLoginHandler(deps))
	mux.HandleFunc("/api/logout", apiLogoutHandler(deps))
	mux.HandleFunc("/api/me", deps.Sessions.requireAuth(apiMeHandler()))

	// Task API (protected)
	mux.HandleFunc("/api/tasks", deps.Sessions.requireAuth(apiTasksHandler(deps)))
	mux.HandleFunc("/api/tasks/", deps.Sessions.requireAuth(apiTaskByIDHandler(deps.Stores)))
	mux.HandleFunc("/api/reorder", deps.Sessions.requireAuth(apiReorderHandler(deps.Stores)))
	mux.HandleFunc("/api/labels", deps.Sessions.requireAuth(apiLabelsHandler(deps)))
	mux.HandleFunc("/api/public-labels", deps.Sessions.requireAuth(apiPublicLabelsHandler(deps.Stores)))

	// Static web
	webRoot, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}
	staticFS := http.FileServer(http.FS(webRoot))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login", "/register":
			serveStaticPage(w, r, webRoot, r.URL.Path+".html")
			return
		}
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
		if firstUser {
			if _, migErr := migrateLegacyTasks(username); migErr != nil {
				fmt.Printf("warning: legacy migration failed for %s: %v\n", username, migErr)
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

// --- Task routes -----------------------------------------------------------

// userStoreOrNil resolves the current user's Store or writes a 500.
func userStoreOrNil(w http.ResponseWriter, r *http.Request, mgr *StoreManager) *Store {
	user := usernameFromRequest(r)
	s, err := mgr.ForUser(user)
	if err != nil {
		http.Error(w, "failed to open store", http.StatusInternalServerError)
		return nil
	}
	return s
}

// aggregatedTasks returns the current user's tasks plus shared tasks from
// other users (those tagged with any of that user's public labels).
func aggregatedTasks(deps serverDeps, currentUser string) ([]TaskView, error) {
	out := []TaskView{}

	me, err := deps.Stores.ForUser(currentUser)
	if err != nil {
		return nil, err
	}
	myPublic := make(map[string]bool)
	for _, l := range me.GetPublicLabels() {
		myPublic[l] = true
	}
	for _, t := range me.List() {
		isPublic := false
		for _, l := range t.Labels {
			if myPublic[l] {
				isPublic = true
				break
			}
		}
		out = append(out, TaskView{Task: t, Owner: currentUser, Public: isPublic})
	}

	for _, other := range deps.Users.Usernames() {
		if other == currentUser {
			continue
		}
		s, err := deps.Stores.ForUser(other)
		if err != nil {
			continue
		}
		otherPublic := make(map[string]bool)
		for _, l := range s.GetPublicLabels() {
			otherPublic[l] = true
		}
		if len(otherPublic) == 0 {
			continue
		}
		for _, t := range s.List() {
			shared := false
			for _, l := range t.Labels {
				if otherPublic[l] {
					shared = true
					break
				}
			}
			if shared {
				out = append(out, TaskView{Task: t, Owner: other, Public: true})
			}
		}
	}
	return out, nil
}

func filterViewsByDone(views []TaskView, done bool) []TaskView {
	out := views[:0:0]
	for _, v := range views {
		if v.Done == done {
			out = append(out, v)
		}
	}
	return out
}

func filterViewsByLabel(views []TaskView, label string) []TaskView {
	out := views[:0:0]
	for _, v := range views {
		if HasLabel(v.Task, label) {
			out = append(out, v)
		}
	}
	return out
}

func sortViews(views []TaskView, mode SortMode) {
	if mode != SortByDue {
		return
	}
	sort.SliceStable(views, func(i, j int) bool {
		a, b := views[i], views[j]
		if a.Done != b.Done {
			return !a.Done
		}
		aHas, bHas := a.DueDate != "", b.DueDate != ""
		if aHas != bHas {
			return aHas
		}
		if aHas && a.DueDate != b.DueDate {
			return a.DueDate < b.DueDate
		}
		if a.Owner != b.Owner {
			return a.Owner < b.Owner
		}
		return a.ID < b.ID
	})
}

func apiTasksHandler(deps serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current := usernameFromRequest(r)

		switch r.Method {
		case http.MethodGet:
			views, err := aggregatedTasks(deps, current)
			if err != nil {
				http.Error(w, "failed to aggregate tasks", http.StatusInternalServerError)
				return
			}
			switch r.URL.Query().Get("status") {
			case "pending":
				views = filterViewsByDone(views, false)
			case "done":
				views = filterViewsByDone(views, true)
			case "", "all":
			default:
				http.Error(w, "invalid status; want one of: pending, done, all", http.StatusBadRequest)
				return
			}
			if label := r.URL.Query().Get("label"); label != "" {
				views = filterViewsByLabel(views, label)
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
			sortViews(views, mode)
			writeJSON(w, http.StatusOK, views)

		case http.MethodPost:
			store, err := deps.Stores.ForUser(current)
			if err != nil {
				http.Error(w, "failed to open store", http.StatusInternalServerError)
				return
			}
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
			writeJSON(w, http.StatusCreated, TaskView{Task: t, Owner: current, Public: store.HasAnyPublicLabel(t)})

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func apiTaskByIDHandler(mgr *StoreManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store := userStoreOrNil(w, r, mgr)
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
			current := usernameFromRequest(r)
			writeJSON(w, http.StatusOK, TaskView{Task: updated, Owner: current, Public: store.HasAnyPublicLabel(updated)})

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
		store := userStoreOrNil(w, r, mgr)
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

func apiLabelsHandler(deps serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		current := usernameFromRequest(r)
		seen := map[string]bool{}
		appendFrom := func(s *Store, onlyPublic bool) {
			for _, t := range s.List() {
				for _, l := range t.Labels {
					if onlyPublic && !s.IsPublic(l) {
						continue
					}
					seen[l] = true
				}
			}
		}
		if me, err := deps.Stores.ForUser(current); err == nil {
			appendFrom(me, false)
		}
		for _, other := range deps.Users.Usernames() {
			if other == current {
				continue
			}
			if s, err := deps.Stores.ForUser(other); err == nil {
				appendFrom(s, true)
			}
		}
		labels := make([]string, 0, len(seen))
		for l := range seen {
			labels = append(labels, l)
		}
		sort.Strings(labels)
		writeJSON(w, http.StatusOK, labels)
	}
}

func apiPublicLabelsHandler(mgr *StoreManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store := userStoreOrNil(w, r, mgr)
		if store == nil {
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, store.GetPublicLabels())

		case http.MethodPut:
			var body struct {
				Labels []string `json:"labels"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			updated, err := store.SetPublicLabels(body.Labels)
			if err != nil {
				writeStoreErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, updated)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
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

