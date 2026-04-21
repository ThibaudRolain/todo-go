package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"todo-go/internal/session"
	"todo-go/internal/task"
	"todo-go/internal/user"
)

type testEnv struct {
	t      *testing.T
	mux    http.Handler
	deps   Deps
	cookie *http.Cookie
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	t.Setenv("TODO_GO_DATA", t.TempDir())

	users, err := user.Open(filepath.Join(t.TempDir(), "users.json"))
	if err != nil {
		t.Fatalf("user.Open: %v", err)
	}
	deps := Deps{
		Users:    users,
		Stores:   task.NewManager(),
		Sessions: session.NewManager(),
	}

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

	return &testEnv{t: t, mux: mux, deps: deps}
}

func (e *testEnv) do(method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if e.cookie != nil {
		req.AddCookie(e.cookie)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec
}

func (e *testEnv) register(username, password string) *httptest.ResponseRecorder {
	rec := e.do(http.MethodPost, "/api/register", map[string]string{"username": username, "password": password})
	if rec.Code == http.StatusCreated {
		for _, c := range rec.Result().Cookies() {
			if c.Name == session.CookieName {
				e.cookie = c
			}
		}
	}
	return rec
}

func (e *testEnv) login(username, password string) *httptest.ResponseRecorder {
	rec := e.do(http.MethodPost, "/api/login", map[string]string{"username": username, "password": password})
	if rec.Code == http.StatusOK {
		for _, c := range rec.Result().Cookies() {
			if c.Name == session.CookieName {
				e.cookie = c
			}
		}
	}
	return rec
}

func (e *testEnv) logout() *httptest.ResponseRecorder {
	rec := e.do(http.MethodPost, "/api/logout", nil)
	e.cookie = nil
	return rec
}

func TestAPI_RegisterAndMe(t *testing.T) {
	env := newTestEnv(t)
	rec := env.register("alice", "password123")
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d — %s", rec.Code, rec.Body.String())
	}
	rec = env.do(http.MethodGet, "/api/me", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: %d", rec.Code)
	}
	var me struct {
		Username string `json:"username"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &me)
	if me.Username != "alice" {
		t.Fatalf("got %q", me.Username)
	}
}

func TestAPI_RegisterValidation(t *testing.T) {
	env := newTestEnv(t)
	cases := []struct {
		name           string
		username, pass string
		wantStatus     int
	}{
		{"short username", "ab", "password123", http.StatusBadRequest},
		{"invalid chars", "Al Ice!", "password123", http.StatusBadRequest},
		{"short password", "alice", "short", http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := env.do(http.MethodPost, "/api/register", map[string]string{"username": c.username, "password": c.pass})
			if rec.Code != c.wantStatus {
				t.Fatalf("want %d, got %d", c.wantStatus, rec.Code)
			}
		})
	}
}

func TestAPI_RegisterConflict(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	rec := env.do(http.MethodPost, "/api/register", map[string]string{"username": "alice", "password": "password123"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", rec.Code)
	}
}

func TestAPI_LoginBadPassword(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	env.logout()
	rec := env.login("alice", "WRONG")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestAPI_UnauthorizedWithoutCookie(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(http.MethodGet, "/api/tasks", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestAPI_LogoutRevokesSession(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	auth := env.cookie
	env.logout()
	env.cookie = auth
	rec := env.do(http.MethodGet, "/api/me", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session should 401, got %d", rec.Code)
	}
}

func TestAPI_PerUserIsolation(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "alice task"})

	env.logout()
	env.register("bob", "password123")
	rec := env.do(http.MethodGet, "/api/tasks", nil)
	var views []TaskView
	_ = json.Unmarshal(rec.Body.Bytes(), &views)
	if len(views) != 0 {
		t.Fatalf("bob should see 0 tasks, got %+v", views)
	}
}

func TestAPI_PublicLabelsShareTasks(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "shared", "labels": []string{"team"}})
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "private"})
	env.do(http.MethodPut, "/api/public-labels", map[string]any{"labels": []string{"team"}})

	env.logout()
	env.register("bob", "password123")
	rec := env.do(http.MethodGet, "/api/tasks", nil)
	var views []TaskView
	_ = json.Unmarshal(rec.Body.Bytes(), &views)
	if len(views) != 1 {
		t.Fatalf("bob should see 1 shared task, got %+v", views)
	}
	if views[0].Owner != "alice" || views[0].Title != "shared" {
		t.Fatalf("want alice's 'shared', got %+v", views[0])
	}
}

func TestAPI_AddWithDueDate(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	rec := env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "x", "due_date": "2026-05-01"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d", rec.Code)
	}
	var created TaskView
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.DueDate != "2026-05-01" {
		t.Fatalf("got %q", created.DueDate)
	}
}

func TestAPI_AddWithLabels(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	rec := env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "x", "labels": []string{"Work", "home"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d", rec.Code)
	}
	var created TaskView
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if !reflect.DeepEqual(created.Labels, []string{"work", "home"}) {
		t.Fatalf("got %v", created.Labels)
	}
}

func TestAPI_ListSortByDue(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "no due"})
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "late", "due_date": "2026-05-10"})
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "early", "due_date": "2026-05-01"})

	rec := env.do(http.MethodGet, "/api/tasks?sort=due", nil)
	var views []TaskView
	_ = json.Unmarshal(rec.Body.Bytes(), &views)
	if views[0].Title != "early" || views[1].Title != "late" || views[2].Title != "no due" {
		t.Fatalf("sort wrong: %+v", views)
	}
}

func TestAPI_ListLabelFilter(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "a", "labels": []string{"work"}})
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "b", "labels": []string{"home"}})

	rec := env.do(http.MethodGet, "/api/tasks?label=work", nil)
	var views []TaskView
	_ = json.Unmarshal(rec.Body.Bytes(), &views)
	if len(views) != 1 || views[0].Title != "a" {
		t.Fatalf("got %+v", views)
	}
}

func TestAPI_PatchSetsDueAndLabels(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "x"})

	if rec := env.do(http.MethodPatch, "/api/tasks/1", map[string]any{"due_date": "2026-05-10"}); rec.Code != http.StatusOK {
		t.Fatalf("due: %d", rec.Code)
	}
	if rec := env.do(http.MethodPatch, "/api/tasks/1", map[string]any{"labels": []string{"home"}}); rec.Code != http.StatusOK {
		t.Fatalf("labels: %d", rec.Code)
	}
}

func TestAPI_Delete(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "a"})
	rec := env.do(http.MethodDelete, "/api/tasks/1", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
}

func TestAPI_Reorder(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	for _, title := range []string{"a", "b", "c"} {
		env.do(http.MethodPost, "/api/tasks", map[string]any{"title": title})
	}
	rec := env.do(http.MethodPost, "/api/reorder", map[string]any{"ids": []int{3, 1, 2}})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestAPI_Labels(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "a", "labels": []string{"work"}})
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "b", "labels": []string{"home", "work"}})

	rec := env.do(http.MethodGet, "/api/labels", nil)
	var labels []string
	_ = json.Unmarshal(rec.Body.Bytes(), &labels)
	if !reflect.DeepEqual(labels, []string{"home", "work"}) {
		t.Fatalf("got %v", labels)
	}
}

func TestAPI_PublicLabelsCRUD(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")

	rec := env.do(http.MethodPut, "/api/public-labels", map[string]any{"labels": []string{"team", "Ops"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d", rec.Code)
	}
	var got []string
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if !reflect.DeepEqual(got, []string{"team", "ops"}) {
		t.Fatalf("got %v", got)
	}

	rec = env.do(http.MethodGet, "/api/public-labels", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if !reflect.DeepEqual(got, []string{"team", "ops"}) {
		t.Fatalf("got %v", got)
	}
}

func TestAPI_BadSortAndStatus(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	if rec := env.do(http.MethodGet, "/api/tasks?sort=nope", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("sort: %d", rec.Code)
	}
	if rec := env.do(http.MethodGet, "/api/tasks?status=bogus", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", rec.Code)
	}
}
