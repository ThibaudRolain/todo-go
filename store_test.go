package main

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
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

	a, err := s.Add("first", "")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	b, err := s.Add("second", "")
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
	if _, err := s.Add("", ""); !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("want ErrEmptyTitle, got %v", err)
	}
}

func TestStore_AddWithDueDate(t *testing.T) {
	s := newTempStore(t)
	task, err := s.Add("buy milk", "2026-05-01")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if task.DueDate != "2026-05-01" {
		t.Fatalf("want due 2026-05-01, got %q", task.DueDate)
	}
}

func TestStore_AddRejectsBadDue(t *testing.T) {
	s := newTempStore(t)
	if _, err := s.Add("x", "not-a-date"); !errors.Is(err, ErrBadDueDate) {
		t.Fatalf("want ErrBadDueDate, got %v", err)
	}
	if _, err := s.Add("x", "2026/05/01"); !errors.Is(err, ErrBadDueDate) {
		t.Fatalf("want ErrBadDueDate for slash format, got %v", err)
	}
}

func TestStore_SetDoneTogglesState(t *testing.T) {
	s := newTempStore(t)
	task, _ := s.Add("buy milk", "")

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
	if _, err := s.SetDone(42, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestStore_SetTitleUpdates(t *testing.T) {
	s := newTempStore(t)
	task, _ := s.Add("old", "")

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
	task, _ := s.Add("old", "")

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

func TestStore_SetDueUpdates(t *testing.T) {
	s := newTempStore(t)
	task, _ := s.Add("x", "")

	updated, err := s.SetDue(task.ID, "2026-06-10")
	if err != nil {
		t.Fatalf("SetDue: %v", err)
	}
	if updated.DueDate != "2026-06-10" {
		t.Fatalf("want 2026-06-10, got %q", updated.DueDate)
	}
}

func TestStore_SetDueClears(t *testing.T) {
	s := newTempStore(t)
	task, _ := s.Add("x", "2026-06-10")

	updated, err := s.SetDue(task.ID, "")
	if err != nil {
		t.Fatalf("SetDue clear: %v", err)
	}
	if updated.DueDate != "" {
		t.Fatalf("want empty due, got %q", updated.DueDate)
	}
}

func TestStore_SetDueRejectsBad(t *testing.T) {
	s := newTempStore(t)
	task, _ := s.Add("x", "")
	if _, err := s.SetDue(task.ID, "yesterday"); !errors.Is(err, ErrBadDueDate) {
		t.Fatalf("want ErrBadDueDate, got %v", err)
	}
}

func TestStore_RemoveDropsTask(t *testing.T) {
	s := newTempStore(t)
	a, _ := s.Add("a", "")
	b, _ := s.Add("b", "")
	c, _ := s.Add("c", "")

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
	s.Add("a", "")

	if err := s.Remove(99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestStore_ReorderChangesOrder(t *testing.T) {
	s := newTempStore(t)
	s.Add("a", "")
	s.Add("b", "")
	s.Add("c", "")

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
	s.Add("a", "")
	s.Add("b", "")
	if err := s.Reorder([]int{1}); !errors.Is(err, ErrReorderLength) {
		t.Fatalf("want ErrReorderLength, got %v", err)
	}
}

func TestStore_ReorderRejectsUnknownID(t *testing.T) {
	s := newTempStore(t)
	s.Add("a", "")
	s.Add("b", "")
	if err := s.Reorder([]int{1, 99}); !errors.Is(err, ErrReorderUnknown) {
		t.Fatalf("want ErrReorderUnknown, got %v", err)
	}
}

func TestStore_ReorderRejectsDuplicates(t *testing.T) {
	s := newTempStore(t)
	s.Add("a", "")
	s.Add("b", "")
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
	s1.Add("a", "")
	s1.Add("b", "")
	s1.SetDone(1, true)
	s1.Remove(2)
	s1.Add("c", "2026-07-01")

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
	if tasks[1].ID != 3 || tasks[1].Title != "c" || tasks[1].DueDate != "2026-07-01" {
		t.Fatalf("second task should be id=3 title=c due=2026-07-01, got %+v", tasks[1])
	}

	next, err := s2.Add("d", "")
	if err != nil {
		t.Fatalf("Add after reopen: %v", err)
	}
	if next.ID != 4 {
		t.Fatalf("want next id 4 after reopen, got %d", next.ID)
	}
}

func TestStore_ListReturnsCopy(t *testing.T) {
	s := newTempStore(t)
	s.Add("a", "")

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

func TestSortTasks_ByDue(t *testing.T) {
	tasks := []Task{
		{ID: 1, Title: "done1", Done: true, DueDate: "2026-01-01"},
		{ID: 2, Title: "no due"},
		{ID: 3, Title: "late", DueDate: "2026-05-01"},
		{ID: 4, Title: "early", DueDate: "2026-04-01"},
		{ID: 5, Title: "done2", Done: true},
	}
	SortTasks(tasks, SortByDue)

	wantOrder := []int{4, 3, 2, 1, 5}
	got := make([]int, len(tasks))
	for i, tk := range tasks {
		got[i] = tk.ID
	}
	for i, id := range wantOrder {
		if got[i] != id {
			t.Fatalf("sort mismatch at %d: want %v, got %v", i, wantOrder, got)
		}
	}
}

func TestSortTasks_Manual(t *testing.T) {
	tasks := []Task{
		{ID: 5, Title: "e"},
		{ID: 1, Title: "a"},
		{ID: 3, Title: "c"},
	}
	SortTasks(tasks, SortManual)
	if tasks[0].ID != 5 || tasks[1].ID != 1 || tasks[2].ID != 3 {
		t.Fatalf("manual sort should not reorder, got %+v", tasks)
	}
}

func TestIsOverdue(t *testing.T) {
	today := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		task Task
		want bool
	}{
		{"past, pending", Task{DueDate: "2026-04-20"}, true},
		{"today, pending", Task{DueDate: "2026-04-21"}, false},
		{"future, pending", Task{DueDate: "2026-04-22"}, false},
		{"past, done", Task{DueDate: "2026-04-20", Done: true}, false},
		{"no due", Task{}, false},
		{"bad date", Task{DueDate: "not-a-date"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsOverdue(c.task, today); got != c.want {
				t.Fatalf("IsOverdue(%+v) = %v, want %v", c.task, got, c.want)
			}
		})
	}
}
