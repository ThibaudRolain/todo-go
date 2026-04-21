package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
)

// testEnv spins up a server-in-process with an authenticated client.
type testEnv struct {
	t        *testing.T
	mux      http.Handler
	sessions *SessionManager
	users    *UserStore
	mgr      *StoreManager
	cookie   *http.Cookie // set after registerAndLogin
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	t.Setenv("TODO_GO_DATA", t.TempDir()) // per-user data goes here

	users, err := OpenUsers(filepath.Join(t.TempDir(), "users.json"))
	if err != nil {
		t.Fatalf("OpenUsers: %v", err)
	}
	deps := serverDeps{
		Users:    users,
		Stores:   NewStoreManager(),
		Sessions: NewSessionManager(),
	}
	env := &testEnv{
		t:        t,
		sessions: deps.Sessions,
		users:    users,
		mgr:      deps.Stores,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/register", apiRegisterHandler(deps))
	mux.HandleFunc("/api/login", apiLoginHandler(deps))
	mux.HandleFunc("/api/logout", apiLogoutHandler(deps))
	mux.HandleFunc("/api/me", deps.Sessions.requireAuth(apiMeHandler()))
	mux.HandleFunc("/api/tasks", deps.Sessions.requireAuth(apiTasksHandler(deps.Stores)))
	mux.HandleFunc("/api/tasks/", deps.Sessions.requireAuth(apiTaskByIDHandler(deps.Stores)))
	mux.HandleFunc("/api/reorder", deps.Sessions.requireAuth(apiReorderHandler(deps.Stores)))
	mux.HandleFunc("/api/labels", deps.Sessions.requireAuth(apiLabelsHandler(deps.Stores)))
	env.mux = mux
	return env
}

func (e *testEnv) do(method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			e.t.Fatalf("encode body: %v", err)
		}
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
			if c.Name == sessionCookieName {
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
			if c.Name == sessionCookieName {
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
	rec := env.register("alice", "s3cretPw!")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: want 201, got %d — %s", rec.Code, rec.Body.String())
	}

	rec = env.do(http.MethodGet, "/api/me", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: want 200, got %d", rec.Code)
	}
	var me struct {
		Username string `json:"username"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &me)
	if me.Username != "alice" {
		t.Fatalf("me.username: want alice, got %q", me.Username)
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
				t.Fatalf("want %d, got %d — %s", c.wantStatus, rec.Code, rec.Body.String())
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

func TestAPI_LoginFlow(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	env.logout()

	rec := env.login("alice", "password123")
	if rec.Code != http.StatusOK {
		t.Fatalf("login: want 200, got %d — %s", rec.Code, rec.Body.String())
	}

	rec = env.do(http.MethodGet, "/api/me", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("me after login: want 200, got %d", rec.Code)
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

	// Keep the cookie so we can re-send it after logout.
	authCookie := env.cookie
	rec := env.logout()
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: want 204, got %d", rec.Code)
	}

	// Use the pre-logout cookie — should now be invalid.
	env.cookie = authCookie
	rec = env.do(http.MethodGet, "/api/me", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("session should be revoked; want 401, got %d", rec.Code)
	}
}

func TestAPI_PerUserIsolation(t *testing.T) {
	env := newTestEnv(t)

	env.register("alice", "password123")
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "alice task"})

	env.logout()
	env.register("bob", "password123")

	rec := env.do(http.MethodGet, "/api/tasks", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var tasks []Task
	_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
	if len(tasks) != 0 {
		t.Fatalf("bob should see 0 tasks, got %d: %+v", len(tasks), tasks)
	}

	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "bob task"})

	env.logout()
	env.login("alice", "password123")
	rec = env.do(http.MethodGet, "/api/tasks", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
	if len(tasks) != 1 || tasks[0].Title != "alice task" {
		t.Fatalf("alice should see only her task, got %+v", tasks)
	}
}

// --- Existing task-route tests, now authenticated via env.register ---------

func TestAPI_AddAndList(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")

	rec := env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "buy milk"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d", rec.Code)
	}
	var created Task
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Title != "buy milk" {
		t.Fatalf("unexpected created: %+v", created)
	}

	rec = env.do(http.MethodGet, "/api/tasks", nil)
	var tasks []Task
	_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
	if len(tasks) != 1 {
		t.Fatalf("want 1 task, got %d", len(tasks))
	}
}

func TestAPI_AddWithDueDate(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	rec := env.do(http.MethodPost, "/api/tasks", map[string]any{
		"title":    "x",
		"due_date": "2026-05-01",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d", rec.Code)
	}
	var created Task
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.DueDate != "2026-05-01" {
		t.Fatalf("got %q", created.DueDate)
	}
}

func TestAPI_AddRejectsBadDue(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	rec := env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "x", "due_date": "not-a-date"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestAPI_AddWithLabels(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	rec := env.do(http.MethodPost, "/api/tasks", map[string]any{
		"title":  "x",
		"labels": []string{"Work", "home"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d", rec.Code)
	}
	var created Task
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
	var tasks []Task
	_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
	if tasks[0].Title != "early" || tasks[1].Title != "late" || tasks[2].Title != "no due" {
		t.Fatalf("sort wrong: %+v", tasks)
	}
}

func TestAPI_ListLabelFilter(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "a", "labels": []string{"work"}})
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "b", "labels": []string{"home"}})

	rec := env.do(http.MethodGet, "/api/tasks?label=work", nil)
	var tasks []Task
	_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
	if len(tasks) != 1 || tasks[0].Title != "a" {
		t.Fatalf("got %+v", tasks)
	}
}

func TestAPI_PatchTitleAndDoneAndLabelsAndDue(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "old"})

	rec := env.do(http.MethodPatch, "/api/tasks/1", map[string]any{"title": "new"})
	if rec.Code != http.StatusOK {
		t.Fatalf("title: %d", rec.Code)
	}

	rec = env.do(http.MethodPatch, "/api/tasks/1", map[string]any{"done": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("done: %d", rec.Code)
	}

	rec = env.do(http.MethodPatch, "/api/tasks/1", map[string]any{"due_date": "2026-05-10"})
	if rec.Code != http.StatusOK {
		t.Fatalf("due: %d", rec.Code)
	}

	rec = env.do(http.MethodPatch, "/api/tasks/1", map[string]any{"labels": []string{"home"}})
	if rec.Code != http.StatusOK {
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
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var labels []string
	_ = json.Unmarshal(rec.Body.Bytes(), &labels)
	if !reflect.DeepEqual(labels, []string{"home", "work"}) {
		t.Fatalf("got %v", labels)
	}
}

func TestAPI_BadSortAndStatus(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	rec := env.do(http.MethodGet, "/api/tasks?sort=nope", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad sort: %d", rec.Code)
	}
	rec = env.do(http.MethodGet, "/api/tasks?status=bogus", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status: %d", rec.Code)
	}
}

func TestAPI_PatchNoFields(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	env.do(http.MethodPost, "/api/tasks", map[string]any{"title": "a"})
	rec := env.do(http.MethodPatch, "/api/tasks/1", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestAPI_MethodNotAllowed(t *testing.T) {
	env := newTestEnv(t)
	env.register("alice", "password123")
	rec := env.do(http.MethodPut, "/api/tasks", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}
