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
	mux.HandleFunc("/api/labels", apiLabelsHandler(store))
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

	rec := doJSON(t, h, http.MethodPost, "/api/tasks", map[string]any{"title": "buy milk"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/tasks: want 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var created Task
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID != 1 || created.Title != "buy milk" {
		t.Fatalf("unexpected created: %+v", created)
	}

	rec = doJSON(t, h, http.MethodGet, "/api/tasks", nil)
	var tasks []Task
	_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
	if len(tasks) != 1 || tasks[0].Title != "buy milk" {
		t.Fatalf("unexpected list: %+v", tasks)
	}
}

func TestAPI_AddWithDueDate(t *testing.T) {
	_, h := newTestServer(t)
	rec := doJSON(t, h, http.MethodPost, "/api/tasks", map[string]any{
		"title":    "buy milk",
		"due_date": "2026-05-01",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var created Task
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.DueDate != "2026-05-01" {
		t.Fatalf("want due 2026-05-01, got %q", created.DueDate)
	}
}

func TestAPI_AddRejectsBadDue(t *testing.T) {
	_, h := newTestServer(t)
	rec := doJSON(t, h, http.MethodPost, "/api/tasks", map[string]any{
		"title":    "x",
		"due_date": "not-a-date",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestAPI_AddWithLabels(t *testing.T) {
	_, h := newTestServer(t)
	rec := doJSON(t, h, http.MethodPost, "/api/tasks", map[string]any{
		"title":  "x",
		"labels": []string{"Work", "home"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var created Task
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if !reflect.DeepEqual(created.Labels, []string{"work", "home"}) {
		t.Fatalf("labels should be normalized; got %v", created.Labels)
	}
}

func TestAPI_AddRejectsBadLabels(t *testing.T) {
	_, h := newTestServer(t)
	rec := doJSON(t, h, http.MethodPost, "/api/tasks", map[string]any{
		"title":  "x",
		"labels": []string{"has space"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestAPI_ListStatusFilter(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "a"})
	s.Add(NewTask{Title: "b"})
	s.SetDone(1, true)

	check := func(path string, want int) {
		t.Helper()
		rec := doJSON(t, h, http.MethodGet, path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: %d", path, rec.Code)
		}
		var tasks []Task
		_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
		if len(tasks) != want {
			t.Fatalf("GET %s: want %d, got %d", path, want, len(tasks))
		}
	}
	check("/api/tasks", 2)
	check("/api/tasks?status=pending", 1)
	check("/api/tasks?status=done", 1)

	rec := doJSON(t, h, http.MethodGet, "/api/tasks?status=bogus", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for bogus status, got %d", rec.Code)
	}
}

func TestAPI_ListLabelFilter(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "a", Labels: []string{"work"}})
	s.Add(NewTask{Title: "b", Labels: []string{"home"}})
	s.Add(NewTask{Title: "c", Labels: []string{"work", "urgent"}})

	rec := doJSON(t, h, http.MethodGet, "/api/tasks?label=work", nil)
	var tasks []Task
	_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
	if len(tasks) != 2 {
		t.Fatalf("want 2 tasks with label work, got %d: %+v", len(tasks), tasks)
	}
}

func TestAPI_ListSortByDue(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "no due"})
	s.Add(NewTask{Title: "late", DueDate: "2026-05-10"})
	s.Add(NewTask{Title: "early", DueDate: "2026-05-01"})

	rec := doJSON(t, h, http.MethodGet, "/api/tasks?sort=due", nil)
	var tasks []Task
	_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
	if tasks[0].Title != "early" || tasks[1].Title != "late" || tasks[2].Title != "no due" {
		t.Fatalf("sort order wrong: %+v", tasks)
	}
}

func TestAPI_ListSortManualPreservesInsertion(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "third", DueDate: "2026-01-01"})
	s.Add(NewTask{Title: "first"})
	s.Add(NewTask{Title: "second", DueDate: "2026-09-09"})

	rec := doJSON(t, h, http.MethodGet, "/api/tasks?sort=manual", nil)
	var tasks []Task
	_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
	if tasks[0].Title != "third" || tasks[1].Title != "first" || tasks[2].Title != "second" {
		t.Fatalf("manual sort should keep insertion order, got %+v", tasks)
	}
}

func TestAPI_ListBadSort(t *testing.T) {
	_, h := newTestServer(t)
	rec := doJSON(t, h, http.MethodGet, "/api/tasks?sort=nope", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestAPI_AddRejectsEmptyTitle(t *testing.T) {
	_, h := newTestServer(t)
	rec := doJSON(t, h, http.MethodPost, "/api/tasks", map[string]any{"title": "   "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestAPI_PatchMarksDone(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "a"})
	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/1", map[string]any{"done": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — %s", rec.Code, rec.Body.String())
	}
	var updated Task
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if !updated.Done {
		t.Fatalf("want Done=true, got %+v", updated)
	}
}

func TestAPI_PatchEditsTitle(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "old"})
	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/1", map[string]any{"title": "new"})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var updated Task
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.Title != "new" {
		t.Fatalf("want 'new', got %q", updated.Title)
	}
}

func TestAPI_PatchSetsDue(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "x"})
	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/1", map[string]any{"due_date": "2026-05-10"})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestAPI_PatchClearsDue(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "x", DueDate: "2026-05-10"})
	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/1", map[string]any{"due_date": ""})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestAPI_PatchRejectsBadDue(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "x"})
	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/1", map[string]any{"due_date": "2026/05/10"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestAPI_PatchSetsLabels(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "x"})

	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/1", map[string]any{"labels": []string{"Work", "urgent"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — %s", rec.Code, rec.Body.String())
	}
	var updated Task
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if !reflect.DeepEqual(updated.Labels, []string{"work", "urgent"}) {
		t.Fatalf("want [work urgent], got %v", updated.Labels)
	}
}

func TestAPI_PatchClearsLabels(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "x", Labels: []string{"work"}})
	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/1", map[string]any{"labels": []string{}})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var updated Task
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if len(updated.Labels) != 0 {
		t.Fatalf("want empty labels, got %v", updated.Labels)
	}
}

func TestAPI_PatchRejectsBadLabels(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "x"})
	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/1", map[string]any{"labels": []string{"has space"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestAPI_PatchRejectsEmptyTitle(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "old"})
	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/1", map[string]any{"title": "   "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestAPI_PatchRejectsNoFields(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "a"})
	rec := doJSON(t, h, http.MethodPatch, "/api/tasks/1", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
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
	s.Add(NewTask{Title: "a"})
	s.Add(NewTask{Title: "b"})

	rec := doJSON(t, h, http.MethodDelete, "/api/tasks/1", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
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
	s.Add(NewTask{Title: "a"})
	s.Add(NewTask{Title: "b"})
	s.Add(NewTask{Title: "c"})

	rec := doJSON(t, h, http.MethodPost, "/api/reorder", map[string]any{"ids": []int{3, 1, 2}})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestAPI_ReorderBadIDs(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "a"})
	s.Add(NewTask{Title: "b"})
	rec := doJSON(t, h, http.MethodPost, "/api/reorder", map[string]any{"ids": []int{1, 99}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestAPI_Labels(t *testing.T) {
	s, h := newTestServer(t)
	s.Add(NewTask{Title: "a", Labels: []string{"work"}})
	s.Add(NewTask{Title: "b", Labels: []string{"home", "work"}})

	rec := doJSON(t, h, http.MethodGet, "/api/labels", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var labels []string
	_ = json.Unmarshal(rec.Body.Bytes(), &labels)
	if !reflect.DeepEqual(labels, []string{"home", "work"}) {
		t.Fatalf("want [home work], got %v", labels)
	}
}
