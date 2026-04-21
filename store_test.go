package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func newTempStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tasks.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	return s
}

func TestStore_AddAssignsSequentialIDs(t *testing.T) {
	s := newTempStore(t)

	a, err := s.Add("first")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	b, err := s.Add("second")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if a.ID != 1 || b.ID != 2 {
		t.Fatalf("want ids 1 and 2, got %d and %d", a.ID, b.ID)
	}
	if a.Title != "first" || a.Done {
		t.Fatalf("unexpected task a: %+v", a)
	}
}

func TestStore_AddRejectsEmpty(t *testing.T) {
	s := newTempStore(t)
	if _, err := s.Add(""); !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("want ErrEmptyTitle, got %v", err)
	}
}

func TestStore_SetDoneTogglesState(t *testing.T) {
	s := newTempStore(t)
	task, _ := s.Add("buy milk")

	updated, err := s.SetDone(task.ID, true)
	if err != nil {
		t.Fatalf("SetDone true: %v", err)
	}
	if !updated.Done {
		t.Fatalf("want Done=true, got false")
	}

	updated, err = s.SetDone(task.ID, false)
	if err != nil {
		t.Fatalf("SetDone false: %v", err)
	}
	if updated.Done {
		t.Fatalf("want Done=false, got true")
	}
}

func TestStore_SetDoneUnknownID(t *testing.T) {
	s := newTempStore(t)

	_, err := s.SetDone(42, true)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestStore_SetTitleUpdates(t *testing.T) {
	s := newTempStore(t)
	task, _ := s.Add("old")

	updated, err := s.SetTitle(task.ID, "new")
	if err != nil {
		t.Fatalf("SetTitle: %v", err)
	}
	if updated.Title != "new" {
		t.Fatalf("want title 'new', got %q", updated.Title)
	}
}

func TestStore_SetTitleRejectsEmpty(t *testing.T) {
	s := newTempStore(t)
	task, _ := s.Add("old")

	if _, err := s.SetTitle(task.ID, ""); !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("want ErrEmptyTitle, got %v", err)
	}
}

func TestStore_SetTitleUnknownID(t *testing.T) {
	s := newTempStore(t)
	if _, err := s.SetTitle(99, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestStore_RemoveDropsTask(t *testing.T) {
	s := newTempStore(t)
	a, _ := s.Add("a")
	b, _ := s.Add("b")
	c, _ := s.Add("c")

	if err := s.Remove(b.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	tasks := s.List()
	if len(tasks) != 2 {
		t.Fatalf("want 2 tasks left, got %d", len(tasks))
	}
	if tasks[0].ID != a.ID || tasks[1].ID != c.ID {
		t.Fatalf("unexpected tasks after remove: %+v", tasks)
	}
}

func TestStore_RemoveUnknownID(t *testing.T) {
	s := newTempStore(t)
	s.Add("a")

	err := s.Remove(99)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestStore_ReorderChangesOrder(t *testing.T) {
	s := newTempStore(t)
	s.Add("a")
	s.Add("b")
	s.Add("c")

	if err := s.Reorder([]int{3, 1, 2}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}

	tasks := s.List()
	ids := []int{tasks[0].ID, tasks[1].ID, tasks[2].ID}
	if ids[0] != 3 || ids[1] != 1 || ids[2] != 2 {
		t.Fatalf("unexpected order after reorder: %+v", ids)
	}
}

func TestStore_ReorderRejectsMismatchedLength(t *testing.T) {
	s := newTempStore(t)
	s.Add("a")
	s.Add("b")
	if err := s.Reorder([]int{1}); !errors.Is(err, ErrReorderLength) {
		t.Fatalf("want ErrReorderLength, got %v", err)
	}
}

func TestStore_ReorderRejectsUnknownID(t *testing.T) {
	s := newTempStore(t)
	s.Add("a")
	s.Add("b")
	if err := s.Reorder([]int{1, 99}); !errors.Is(err, ErrReorderUnknown) {
		t.Fatalf("want ErrReorderUnknown, got %v", err)
	}
}

func TestStore_ReorderRejectsDuplicates(t *testing.T) {
	s := newTempStore(t)
	s.Add("a")
	s.Add("b")
	if err := s.Reorder([]int{1, 1}); !errors.Is(err, ErrReorderUnknown) {
		t.Fatalf("want ErrReorderUnknown (duplicate), got %v", err)
	}
}

func TestStore_PersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")

	s1, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore 1: %v", err)
	}
	s1.Add("a")
	s1.Add("b")
	s1.SetDone(1, true)
	s1.Remove(2)
	s1.Add("c")

	s2, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore 2: %v", err)
	}
	tasks := s2.List()
	if len(tasks) != 2 {
		t.Fatalf("want 2 tasks after reopen, got %d: %+v", len(tasks), tasks)
	}
	if tasks[0].ID != 1 || !tasks[0].Done {
		t.Fatalf("first task should be id=1 done=true, got %+v", tasks[0])
	}
	if tasks[1].ID != 3 || tasks[1].Title != "c" {
		t.Fatalf("second task should be id=3 title=c, got %+v", tasks[1])
	}

	next, err := s2.Add("d")
	if err != nil {
		t.Fatalf("Add after reopen: %v", err)
	}
	if next.ID != 4 {
		t.Fatalf("want next id 4 after reopen, got %d", next.ID)
	}
}

func TestStore_ListReturnsCopy(t *testing.T) {
	s := newTempStore(t)
	s.Add("a")

	got := s.List()
	got[0].Title = "mutated"

	fresh := s.List()
	if fresh[0].Title != "a" {
		t.Fatalf("List() must return a copy; store was mutated via caller")
	}
}

func TestFilterByDone(t *testing.T) {
	tasks := []Task{
		{ID: 1, Title: "a", Done: true},
		{ID: 2, Title: "b", Done: false},
		{ID: 3, Title: "c", Done: true},
	}
	pending := filterByDone(tasks, false)
	if len(pending) != 1 || pending[0].ID != 2 {
		t.Fatalf("pending filter wrong: %+v", pending)
	}
	done := filterByDone(tasks, true)
	if len(done) != 2 || done[0].ID != 1 || done[1].ID != 3 {
		t.Fatalf("done filter wrong: %+v", done)
	}
}
