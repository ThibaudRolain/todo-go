package task

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
	if _, err := s.Add(NewTask{}); !errors.Is(err, ErrEmptyTitle) {
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
		t.Fatalf("want %v, got %v", want, task.Labels)
	}
}

func TestStore_AddRejectsBadLabels(t *testing.T) {
	s := newTempStore(t)
	for _, c := range [][]string{{""}, {"   "}, {"has space"}, {"x\ty"}} {
		if _, err := s.Add(NewTask{Title: "x", Labels: c}); !errors.Is(err, ErrBadLabel) {
			t.Fatalf("want ErrBadLabel for %v, got %v", c, err)
		}
	}
}

func TestStore_SetDoneTogglesState(t *testing.T) {
	s := newTempStore(t)
	task := add(t, s, "buy milk")
	updated, err := s.SetDone(task.ID, true)
	if err != nil || !updated.Done {
		t.Fatalf("SetDone true: %v %+v", err, updated)
	}
	updated, _ = s.SetDone(task.ID, false)
	if updated.Done {
		t.Fatalf("want Done=false")
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
	if err != nil || updated.Title != "new" {
		t.Fatalf("SetTitle: %v, %+v", err, updated)
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
	if err != nil || updated.DueDate != "2026-06-10" {
		t.Fatalf("SetDue: %v, %+v", err, updated)
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

func TestStore_SetLabelsReplaces(t *testing.T) {
	s := newTempStore(t)
	task, _ := s.Add(NewTask{Title: "x", Labels: []string{"work"}})
	updated, err := s.SetLabels(task.ID, []string{"Home", "urgent", "home"})
	if err != nil {
		t.Fatalf("SetLabels: %v", err)
	}
	if !reflect.DeepEqual(updated.Labels, []string{"home", "urgent"}) {
		t.Fatalf("got %v", updated.Labels)
	}
}

func TestStore_AddLabelAppends(t *testing.T) {
	s := newTempStore(t)
	task := add(t, s, "x")
	updated, _ := s.AddLabel(task.ID, "Work")
	if !reflect.DeepEqual(updated.Labels, []string{"work"}) {
		t.Fatalf("want [work], got %v", updated.Labels)
	}
	updated, _ = s.AddLabel(task.ID, "work")
	if !reflect.DeepEqual(updated.Labels, []string{"work"}) {
		t.Fatalf("dedupe failed: %v", updated.Labels)
	}
	updated, _ = s.AddLabel(task.ID, "home")
	if !reflect.DeepEqual(updated.Labels, []string{"work", "home"}) {
		t.Fatalf("want [work home], got %v", updated.Labels)
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
		t.Fatalf("got %v", updated.Labels)
	}
}

func TestStore_PublicLabels(t *testing.T) {
	s := newTempStore(t)
	if _, err := s.AddPublicLabel("work"); err != nil {
		t.Fatalf("AddPublicLabel: %v", err)
	}
	if !s.IsPublic("work") {
		t.Fatalf("IsPublic should be true")
	}
	if _, err := s.AddPublicLabel("work"); err != nil {
		t.Fatalf("dedupe should not error: %v", err)
	}
	if got := s.GetPublicLabels(); !reflect.DeepEqual(got, []string{"work"}) {
		t.Fatalf("want [work], got %v", got)
	}
	if _, err := s.RemovePublicLabel("work"); err != nil {
		t.Fatalf("RemovePublicLabel: %v", err)
	}
	if s.IsPublic("work") {
		t.Fatalf("IsPublic should be false after remove")
	}
}

func TestStore_HasAnyPublicLabel(t *testing.T) {
	s := newTempStore(t)
	s.AddPublicLabel("team")
	taskPrivate := Task{Labels: []string{"home"}}
	taskPublic := Task{Labels: []string{"home", "team"}}
	if s.HasAnyPublicLabel(taskPrivate) {
		t.Fatalf("private task should not match")
	}
	if !s.HasAnyPublicLabel(taskPublic) {
		t.Fatalf("public task should match")
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
		t.Fatalf("unexpected order: %+v", tasks)
	}
}

func TestStore_ReorderRejectsBadInput(t *testing.T) {
	s := newTempStore(t)
	add(t, s, "a")
	add(t, s, "b")
	if err := s.Reorder([]int{1}); !errors.Is(err, ErrReorderLength) {
		t.Fatalf("want ErrReorderLength, got %v", err)
	}
	if err := s.Reorder([]int{1, 99}); !errors.Is(err, ErrReorderUnknown) {
		t.Fatalf("want ErrReorderUnknown, got %v", err)
	}
	if err := s.Reorder([]int{1, 1}); !errors.Is(err, ErrReorderUnknown) {
		t.Fatalf("want ErrReorderUnknown for duplicate, got %v", err)
	}
}

func TestStore_PersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	s1, _ := OpenStore(path)
	s1.Add(NewTask{Title: "a", Labels: []string{"work"}})
	s1.Add(NewTask{Title: "b"})
	s1.SetDone(1, true)
	s1.Remove(2)
	s1.Add(NewTask{Title: "c", DueDate: "2026-07-01", Labels: []string{"home"}})
	s1.AddPublicLabel("home")

	s2, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore 2: %v", err)
	}
	tasks := s2.List()
	if len(tasks) != 2 {
		t.Fatalf("want 2 tasks after reopen, got %d: %+v", len(tasks), tasks)
	}
	if !s2.IsPublic("home") {
		t.Fatalf("public label should persist")
	}
	next, _ := s2.Add(NewTask{Title: "d"})
	if next.ID != 4 {
		t.Fatalf("want next id 4, got %d", next.ID)
	}
}

func TestStore_ListReturnsCopy(t *testing.T) {
	s := newTempStore(t)
	add(t, s, "a")
	got := s.List()
	got[0].Title = "mutated"
	fresh := s.List()
	if fresh[0].Title != "a" {
		t.Fatalf("List must return a copy")
	}
}

func TestFilterByDone(t *testing.T) {
	tasks := []Task{{ID: 1, Done: true}, {ID: 2}, {ID: 3, Done: true}}
	pending := FilterByDone(tasks, false)
	if len(pending) != 1 || pending[0].ID != 2 {
		t.Fatalf("pending: %+v", pending)
	}
}

func TestFilterByLabel(t *testing.T) {
	tasks := []Task{
		{ID: 1, Labels: []string{"work"}},
		{ID: 2, Labels: []string{"home"}},
		{ID: 3, Labels: []string{"work", "urgent"}},
	}
	got := FilterByLabel(tasks, "WORK")
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 {
		t.Fatalf("got %+v", got)
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
	want := []int{4, 3, 2, 1, 5}
	for i, id := range want {
		if tasks[i].ID != id {
			t.Fatalf("want %v", want)
		}
	}
}

func TestSortTasks_Manual(t *testing.T) {
	tasks := []Task{{ID: 5}, {ID: 1}, {ID: 3}}
	SortTasks(tasks, SortManual)
	if tasks[0].ID != 5 || tasks[1].ID != 1 || tasks[2].ID != 3 {
		t.Fatalf("manual should not reorder")
	}
}

func TestIsOverdue(t *testing.T) {
	today := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		task Task
		want bool
	}{
		{"past", Task{DueDate: "2026-04-20"}, true},
		{"today", Task{DueDate: "2026-04-21"}, false},
		{"future", Task{DueDate: "2026-04-22"}, false},
		{"past done", Task{DueDate: "2026-04-20", Done: true}, false},
		{"no due", Task{}, false},
		{"bad date", Task{DueDate: "not-a-date"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsOverdue(c.task, today); got != c.want {
				t.Fatalf("IsOverdue = %v, want %v", got, c.want)
			}
		})
	}
}

func TestHasLabel(t *testing.T) {
	task := Task{Labels: []string{"work", "urgent"}}
	if !HasLabel(task, "work") || !HasLabel(task, "WORK") {
		t.Fatalf("case-insensitive")
	}
	if HasLabel(task, "home") {
		t.Fatalf("missing label")
	}
}

func TestManager_CachesPerUser(t *testing.T) {
	t.Setenv("TODO_GO_DATA", t.TempDir())
	m := NewManager()
	a1, _ := m.ForUser("alice")
	a2, _ := m.ForUser("alice")
	if a1 != a2 {
		t.Fatalf("ForUser should cache per user")
	}
	b, _ := m.ForUser("bob")
	if a1 == b {
		t.Fatalf("different users should get different stores")
	}
}
