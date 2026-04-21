package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func newTestServer(t *testing.T) (*Store, http.Handler) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tasks.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	return s, buildMux(s)
}

func buildMux(store *Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tasks", apiTasksHandler(store))
	mux.HandleFunc("/api/tasks/", apiTaskByIDHandler(store))
	mux.HandleFunc("/api/reorder", apiReorderHandler(store))
	return mux
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAPI_AddAndList(t *testing.T) {
	_, h := newTestServer(t)

	rec := doJSON(t, h, http.MethodPost, "/api/tasks", map[string]string{"title": "buy milk"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/tasks: want 201, got %d — body: %s", rec.Code, rec.Body.String())
	}

	var created Task
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.ID != 1 || created.Title != "buy milk" || created.Done {
		t.Fatalf("unexpected created task: %+v", created)
	}

	rec = doJSON(t, h, http.MethodGet, "/api/tasks", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/tasks: want 200, got %d", rec.Code)
	}
	var tasks []Task
	if err := json.Unmarshal(rec.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != "buy milk" {
		t.Fatalf("unexpected list: %+v", tasks)
	}
}

func TestAPI_ListStatusFilter(t *testing.T) {
	s, h := newTestServer(t)
	s.Add("a")
	s.Add("b")
	s.SetDone(1, true)

	assertListLen := func(path string, want int) {
		t.Helper()
		rec := doJSON(t, h, http.MethodGet, path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: want 200, got %d", path, rec.Code)
		}
		var tasks []Task
		_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
		if len(tasks) != want {
			t.Fatalf("GET %s: want %d tasks, got %d", path, want, len(tasks))
		}
	}
	assertListLen("/api/tasks", 2)
	assertListLen("/api/tasks?status=all", 2)
	assertListLen("/api/tasks?status=pending", 1)
	assertListLen("/api/tasks?status=done", 1)

	rec := doJSON(t, h, http.MethodGet, "/api/tasks?status=bogus", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for bogus status, got %d", rec.Code)
	}
}

func TestAPI_AddRejectsEmptyTitle(t *testing.T) {
	_, h := newTestServer(t)
	rec := doJSON(t, h, http.MethodPost, "/api/tasks", map[string]string{"title": "   "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for empty title, got %d", rec.Code)
	}
}

func TestAPI_PatchMarksDone(t *testing.T) {
	s, h := newTestServer(t)
	s.Add("a")

	done := true
	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/1", map[string]any{"done": done})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	var updated Task
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !updated.Done {
		t.Fatalf("want Done=true, got %+v", updated)
	}
}

func TestAPI_PatchEditsTitle(t *testing.T) {
	s, h := newTestServer(t)
	s.Add("old")

	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/1", map[string]any{"title": "new"})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	var updated Task
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.Title != "new" {
		t.Fatalf("want title 'new', got %q", updated.Title)
	}
}

func TestAPI_PatchRejectsEmptyTitle(t *testing.T) {
	s, h := newTestServer(t)
	s.Add("old")

	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/1", map[string]any{"title": "   "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for empty title, got %d", rec.Code)
	}
}

func TestAPI_PatchRejectsNoFields(t *testing.T) {
	s, h := newTestServer(t)
	s.Add("a")

	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/1", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for empty patch body, got %d", rec.Code)
	}
}

func TestAPI_PatchUnknownID(t *testing.T) {
	_, h := newTestServer(t)
	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/99", map[string]any{"done": true})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestAPI_DeleteRemoves(t *testing.T) {
	s, h := newTestServer(t)
	s.Add("a")
	s.Add("b")

	rec := doJSON(t, h, http.MethodDelete, "/api/tasks/1", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}

	rec = doJSON(t, h, http.MethodGet, "/api/tasks", nil)
	var tasks []Task
	_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
	if len(tasks) != 1 || tasks[0].ID != 2 {
		t.Fatalf("after delete, want [id=2], got %+v", tasks)
	}
}

func TestAPI_BadIDReturns400(t *testing.T) {
	_, h := newTestServer(t)
	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/abc", map[string]any{"done": true})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestAPI_MethodNotAllowed(t *testing.T) {
	_, h := newTestServer(t)
	rec := doJSON(t, h, http.MethodPut, "/api/tasks", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}

func TestAPI_Reorder(t *testing.T) {
	s, h := newTestServer(t)
	s.Add("a")
	s.Add("b")
	s.Add("c")

	rec := doJSON(t, h, http.MethodPost, "/api/reorder", map[string]any{"ids": []int{3, 1, 2}})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	var tasks []Task
	_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
	if len(tasks) != 3 || tasks[0].ID != 3 || tasks[1].ID != 1 || tasks[2].ID != 2 {
		t.Fatalf("unexpected reordered list: %+v", tasks)
	}
}

func TestAPI_ReorderBadIDs(t *testing.T) {
	s, h := newTestServer(t)
	s.Add("a")
	s.Add("b")

	rec := doJSON(t, h, http.MethodPost, "/api/reorder", map[string]any{"ids": []int{1, 99}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}
