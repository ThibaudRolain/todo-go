package main

import (
	"errors"
	"path/filepath"
	"reflect"
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

func add(t *testing.T, s *Store, title string) Task {
	t.Helper()
	task, err := s.Add(NewTask{Title: title})
	if err != nil {
		t.Fatalf("Add %q: %v", title, err)
	}
	return task
}

func TestStore_AddAssignsSequentialIDs(t *testing.T) {
	s := newTempStore(t)
	a := add(t, s, "first")
	b := add(t, s, "second")
	if a.ID != 1 || b.ID != 2 {
		t.Fatalf("want ids 1 and 2, got %d and %d", a.ID, b.ID)
	}
}

func TestStore_AddRejectsEmpty(t *testing.T) {
	s := newTempStore(t)
	if _, err := s.Add(NewTask{Title: ""}); !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("want ErrEmptyTitle, got %v", err)
	}
	if _, err := s.Add(NewTask{Title: "   "}); !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("want ErrEmptyTitle for whitespace, got %v", err)
	}
}

func TestStore_AddWithDueDate(t *testing.T) {
	s := newTempStore(t)
	task, err := s.Add(NewTask{Title: "buy milk", DueDate: "2026-05-01"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if task.DueDate != "2026-05-01" {
		t.Fatalf("want due 2026-05-01, got %q", task.DueDate)
	}
}

func TestStore_AddRejectsBadDue(t *testing.T) {
	s := newTempStore(t)
	if _, err := s.Add(NewTask{Title: "x", DueDate: "not-a-date"}); !errors.Is(err, ErrBadDueDate) {
		t.Fatalf("want ErrBadDueDate, got %v", err)
	}
}

func TestStore_AddWithLabels(t *testing.T) {
	s := newTempStore(t)
	task, err := s.Add(NewTask{Title: "x", Labels: []string{"Work", "home", "WORK"}})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	want := []string{"work", "home"}
	if !reflect.DeepEqual(task.Labels, want) {
		t.Fatalf("labels: want %v (deduped + lowercased, first-seen order), got %v", want, task.Labels)
	}
}

func TestStore_AddRejectsBadLabels(t *testing.T) {
	s := newTempStore(t)
	cases := []struct{ name string; labels []string }{
		{"empty", []string{""}},
		{"whitespace", []string{"   "}},
		{"space inside", []string{"has space"}},
		{"tab inside", []string{"x\ty"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := s.Add(NewTask{Title: "x", Labels: c.labels}); !errors.Is(err, ErrBadLabel) {
				t.Fatalf("want ErrBadLabel, got %v", err)
			}
		})
	}
}

func TestStore_SetDoneTogglesState(t *testing.T) {
	s := newTempStore(t)
	task := add(t, s, "buy milk")

	updated, err := s.SetDone(task.ID, true)
	if err != nil {
		t.Fatalf("SetDone true: %v", err)
	}
	if !updated.Done {
		t.Fatalf("want Done=true, got false")
	}

	updated, _ = s.SetDone(task.ID, false)
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
	task := add(t, s, "old")
	updated, err := s.SetTitle(task.ID, "new")
	if err != nil {
		t.Fatalf("SetTitle: %v", err)
	}
	if updated.Title != "new" {
		t.Fatalf("want 'new', got %q", updated.Title)
	}
}

func TestStore_SetTitleRejectsEmpty(t *testing.T) {
	s := newTempStore(t)
	task := add(t, s, "old")
	if _, err := s.SetTitle(task.ID, ""); !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("want ErrEmptyTitle, got %v", err)
	}
}

func TestStore_SetDueUpdates(t *testing.T) {
	s := newTempStore(t)
	task := add(t, s, "x")
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
	task, _ := s.Add(NewTask{Title: "x", DueDate: "2026-06-10"})
	updated, _ := s.SetDue(task.ID, "")
	if updated.DueDate != "" {
		t.Fatalf("want empty due, got %q", updated.DueDate)
	}
}

func TestStore_SetDueRejectsBad(t *testing.T) {
	s := newTempStore(t)
	task := add(t, s, "x")
	if _, err := s.SetDue(task.ID, "yesterday"); !errors.Is(err, ErrBadDueDate) {
		t.Fatalf("want ErrBadDueDate, got %v", err)
	}
}

func TestStore_SetLabelsReplaces(t *testing.T) {
	s := newTempStore(t)
	task, _ := s.Add(NewTask{Title: "x", Labels: []string{"work"}})

	updated, err := s.SetLabels(task.ID, []string{"Home", "urgent", "home"})
	if err != nil {
		t.Fatalf("SetLabels: %v", err)
	}
	want := []string{"home", "urgent"}
	if !reflect.DeepEqual(updated.Labels, want) {
		t.Fatalf("want %v, got %v", want, updated.Labels)
	}
}

func TestStore_SetLabelsEmptyClears(t *testing.T) {
	s := newTempStore(t)
	task, _ := s.Add(NewTask{Title: "x", Labels: []string{"work"}})
	updated, err := s.SetLabels(task.ID, nil)
	if err != nil {
		t.Fatalf("SetLabels nil: %v", err)
	}
	if len(updated.Labels) != 0 {
		t.Fatalf("want empty labels, got %v", updated.Labels)
	}
}

func TestStore_AddLabelAppends(t *testing.T) {
	s := newTempStore(t)
	task := add(t, s, "x")

	updated, err := s.AddLabel(task.ID, "Work")
	if err != nil {
		t.Fatalf("AddLabel: %v", err)
	}
	if !reflect.DeepEqual(updated.Labels, []string{"work"}) {
		t.Fatalf("want [work], got %v", updated.Labels)
	}

	// Adding again is a no-op (dedupe).
	updated, _ = s.AddLabel(task.ID, "work")
	if !reflect.DeepEqual(updated.Labels, []string{"work"}) {
		t.Fatalf("dedupe failed: got %v", updated.Labels)
	}

	updated, _ = s.AddLabel(task.ID, "home")
	if !reflect.DeepEqual(updated.Labels, []string{"work", "home"}) {
		t.Fatalf("want [work home], got %v", updated.Labels)
	}
}

func TestStore_AddLabelRejectsBad(t *testing.T) {
	s := newTempStore(t)
	task := add(t, s, "x")
	if _, err := s.AddLabel(task.ID, "has space"); !errors.Is(err, ErrBadLabel) {
		t.Fatalf("want ErrBadLabel, got %v", err)
	}
}

func TestStore_RemoveLabelDrops(t *testing.T) {
	s := newTempStore(t)
	task, _ := s.Add(NewTask{Title: "x", Labels: []string{"work", "home"}})
	updated, err := s.RemoveLabel(task.ID, "work")
	if err != nil {
		t.Fatalf("RemoveLabel: %v", err)
	}
	if !reflect.DeepEqual(updated.Labels, []string{"home"}) {
		t.Fatalf("want [home], got %v", updated.Labels)
	}
}

func TestStore_RemoveLabelMissing(t *testing.T) {
	s := newTempStore(t)
	task, _ := s.Add(NewTask{Title: "x", Labels: []string{"work"}})
	updated, err := s.RemoveLabel(task.ID, "nope")
	if err != nil {
		t.Fatalf("RemoveLabel missing: %v", err)
	}
	if !reflect.DeepEqual(updated.Labels, []string{"work"}) {
		t.Fatalf("labels shouldn't change when removing a non-existent label, got %v", updated.Labels)
	}
}

func TestStore_LabelsCollectsDistinct(t *testing.T) {
	s := newTempStore(t)
	s.Add(NewTask{Title: "a", Labels: []string{"work", "home"}})
	s.Add(NewTask{Title: "b", Labels: []string{"work", "urgent"}})
	s.Add(NewTask{Title: "c"})

	got := s.Labels()
	want := []string{"home", "urgent", "work"} // sorted
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestStore_RemoveDropsTask(t *testing.T) {
	s := newTempStore(t)
	a := add(t, s, "a")
	b := add(t, s, "b")
	c := add(t, s, "c")

	if err := s.Remove(b.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	tasks := s.List()
	if len(tasks) != 2 || tasks[0].ID != a.ID || tasks[1].ID != c.ID {
		t.Fatalf("unexpected tasks after remove: %+v", tasks)
	}
}

func TestStore_RemoveUnknownID(t *testing.T) {
	s := newTempStore(t)
	add(t, s, "a")
	if err := s.Remove(99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestStore_ReorderChangesOrder(t *testing.T) {
	s := newTempStore(t)
	add(t, s, "a")
	add(t, s, "b")
	add(t, s, "c")

	if err := s.Reorder([]int{3, 1, 2}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	tasks := s.List()
	if tasks[0].ID != 3 || tasks[1].ID != 1 || tasks[2].ID != 2 {
		t.Fatalf("unexpected order after reorder: %+v", tasks)
	}
}

func TestStore_ReorderRejectsMismatchedLength(t *testing.T) {
	s := newTempStore(t)
	add(t, s, "a")
	add(t, s, "b")
	if err := s.Reorder([]int{1}); !errors.Is(err, ErrReorderLength) {
		t.Fatalf("want ErrReorderLength, got %v", err)
	}
}

func TestStore_ReorderRejectsUnknownID(t *testing.T) {
	s := newTempStore(t)
	add(t, s, "a")
	add(t, s, "b")
	if err := s.Reorder([]int{1, 99}); !errors.Is(err, ErrReorderUnknown) {
		t.Fatalf("want ErrReorderUnknown, got %v", err)
	}
}

func TestStore_ReorderRejectsDuplicates(t *testing.T) {
	s := newTempStore(t)
	add(t, s, "a")
	add(t, s, "b")
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
	s1.Add(NewTask{Title: "a", Labels: []string{"work"}})
	s1.Add(NewTask{Title: "b"})
	s1.SetDone(1, true)
	s1.Remove(2)
	s1.Add(NewTask{Title: "c", DueDate: "2026-07-01", Labels: []string{"home"}})

	s2, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore 2: %v", err)
	}
	tasks := s2.List()
	if len(tasks) != 2 {
		t.Fatalf("want 2 tasks after reopen, got %d: %+v", len(tasks), tasks)
	}
	if tasks[0].ID != 1 || !tasks[0].Done || !reflect.DeepEqual(tasks[0].Labels, []string{"work"}) {
		t.Fatalf("first task wrong after reopen: %+v", tasks[0])
	}
	if tasks[1].ID != 3 || tasks[1].Title != "c" || tasks[1].DueDate != "2026-07-01" || !reflect.DeepEqual(tasks[1].Labels, []string{"home"}) {
		t.Fatalf("second task wrong after reopen: %+v", tasks[1])
	}

	next, _ := s2.Add(NewTask{Title: "d"})
	if next.ID != 4 {
		t.Fatalf("want next id 4 after reopen, got %d", next.ID)
	}
}

func TestStore_ListReturnsCopy(t *testing.T) {
	s := newTempStore(t)
	add(t, s, "a")

	got := s.List()
	got[0].Title = "mutated"
	fresh := s.List()
	if fresh[0].Title != "a" {
		t.Fatalf("List() must return a copy; store was mutated via caller")
	}
}

func TestFilterByDone(t *testing.T) {
	tasks := []Task{
		{ID: 1, Done: true},
		{ID: 2, Done: false},
		{ID: 3, Done: true},
	}
	pending := filterByDone(tasks, false)
	if len(pending) != 1 || pending[0].ID != 2 {
		t.Fatalf("pending filter wrong: %+v", pending)
	}
	done := filterByDone(tasks, true)
	if len(done) != 2 {
		t.Fatalf("done filter wrong: %+v", done)
	}
}

func TestFilterByLabel(t *testing.T) {
	tasks := []Task{
		{ID: 1, Labels: []string{"work"}},
		{ID: 2, Labels: []string{"home"}},
		{ID: 3, Labels: []string{"work", "urgent"}},
	}
	got := filterByLabel(tasks, "work")
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 {
		t.Fatalf("want [1 3] for work, got %+v", got)
	}
	got = filterByLabel(tasks, "WORK")
	if len(got) != 2 {
		t.Fatalf("label filter should be case-insensitive, got %+v", got)
	}
	got = filterByLabel(tasks, "nope")
	if len(got) != 0 {
		t.Fatalf("want none for unknown label, got %+v", got)
	}
}

func TestSortTasks_ByDue(t *testing.T) {
	tasks := []Task{
		{ID: 1, Done: true, DueDate: "2026-01-01"},
		{ID: 2},
		{ID: 3, DueDate: "2026-05-01"},
		{ID: 4, DueDate: "2026-04-01"},
		{ID: 5, Done: true},
	}
	SortTasks(tasks, SortByDue)
	wantOrder := []int{4, 3, 2, 1, 5}
	for i, id := range wantOrder {
		if tasks[i].ID != id {
			t.Fatalf("sort mismatch at %d: want %v", i, wantOrder)
		}
	}
}

func TestSortTasks_Manual(t *testing.T) {
	tasks := []Task{{ID: 5}, {ID: 1}, {ID: 3}}
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

func TestHasLabel(t *testing.T) {
	task := Task{Labels: []string{"work", "urgent"}}
	if !HasLabel(task, "work") || !HasLabel(task, "WORK") {
		t.Fatalf("HasLabel should be case-insensitive")
	}
	if HasLabel(task, "home") {
		t.Fatalf("HasLabel should be false for missing label")
	}
}
