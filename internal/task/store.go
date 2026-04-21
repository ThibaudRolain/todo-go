package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
)

type Store struct {
	mu           sync.Mutex
	path         string
	nextID       int
	tasks        []Task
	publicLabels []string
	publicSet    map[string]bool
}

type storeJSON struct {
	NextID       int      `json:"next_id"`
	Tasks        []Task   `json:"tasks"`
	PublicLabels []string `json:"public_labels,omitempty"`
}

func (s *Store) MarshalJSON() ([]byte, error) {
	return json.Marshal(storeJSON{
		NextID:       s.nextID,
		Tasks:        s.tasks,
		PublicLabels: s.publicLabels,
	})
}

func (s *Store) UnmarshalJSON(data []byte) error {
	var x storeJSON
	if err := json.Unmarshal(data, &x); err != nil {
		return err
	}
	s.nextID = x.NextID
	s.tasks = x.Tasks
	s.publicLabels = x.PublicLabels
	if s.nextID == 0 {
		s.nextID = 1
	}
	if s.tasks == nil {
		s.tasks = []Task{}
	}
	s.rebuildPublicSet()
	return nil
}

func (s *Store) rebuildPublicSet() {
	s.publicSet = make(map[string]bool, len(s.publicLabels))
	for _, l := range s.publicLabels {
		s.publicSet[l] = true
	}
}

func OpenStore(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("OpenStore: empty path")
	}
	s := &Store{path: path, nextID: 1, tasks: []Task{}, publicSet: map[string]bool{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return s, nil
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

func (s *Store) List() []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.tasks)
}

func (s *Store) Labels() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]bool)
	for _, t := range s.tasks {
		for _, l := range t.Labels {
			seen[l] = true
		}
	}
	out := make([]string, 0, len(seen))
	for l := range seen {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

func (s *Store) Add(n NewTask) (Task, error) {
	title := strings.TrimSpace(n.Title)
	if title == "" {
		return Task{}, ErrEmptyTitle
	}
	if err := validateDueDate(n.DueDate); err != nil {
		return Task{}, err
	}
	labels, err := normalizeLabels(n.Labels)
	if err != nil {
		return Task{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := Task{ID: s.nextID, Title: title, DueDate: n.DueDate, Labels: labels}
	s.nextID++
	s.tasks = append(s.tasks, t)
	if err := s.save(); err != nil {
		return Task{}, err
	}
	return t, nil
}

func (s *Store) mutate(id int, fn func(*Task) error) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.tasks {
		if s.tasks[i].ID == id {
			if err := fn(&s.tasks[i]); err != nil {
				return Task{}, err
			}
			if err := s.save(); err != nil {
				return Task{}, err
			}
			return s.tasks[i], nil
		}
	}
	return Task{}, ErrNotFound
}

func (s *Store) SetDone(id int, done bool) (Task, error) {
	return s.mutate(id, func(t *Task) error {
		t.Done = done
		return nil
	})
}

func (s *Store) SetTitle(id int, title string) (Task, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Task{}, ErrEmptyTitle
	}
	return s.mutate(id, func(t *Task) error {
		t.Title = title
		return nil
	})
}

func (s *Store) SetDue(id int, dueDate string) (Task, error) {
	if err := validateDueDate(dueDate); err != nil {
		return Task{}, err
	}
	return s.mutate(id, func(t *Task) error {
		t.DueDate = dueDate
		return nil
	})
}

func (s *Store) SetLabels(id int, labels []string) (Task, error) {
	normalized, err := normalizeLabels(labels)
	if err != nil {
		return Task{}, err
	}
	return s.mutate(id, func(t *Task) error {
		t.Labels = normalized
		return nil
	})
}

func (s *Store) AddLabel(id int, label string) (Task, error) {
	l, err := validateLabel(label)
	if err != nil {
		return Task{}, err
	}
	return s.mutate(id, func(t *Task) error {
		for _, existing := range t.Labels {
			if existing == l {
				return nil
			}
		}
		t.Labels = append(t.Labels, l)
		return nil
	})
}

func (s *Store) RemoveLabel(id int, label string) (Task, error) {
	l, err := validateLabel(label)
	if err != nil {
		return Task{}, err
	}
	return s.mutate(id, func(t *Task) error {
		kept := make([]string, 0, len(t.Labels))
		for _, existing := range t.Labels {
			if existing != l {
				kept = append(kept, existing)
			}
		}
		t.Labels = kept
		return nil
	})
}

func (s *Store) Update(id int, p Patch) (Task, error) {
	var normalizedLabels []string
	var trimmedTitle string
	var trimmedDue string

	if p.Title != nil {
		trimmedTitle = strings.TrimSpace(*p.Title)
		if trimmedTitle == "" {
			return Task{}, ErrEmptyTitle
		}
	}
	if p.DueDate != nil {
		trimmedDue = strings.TrimSpace(*p.DueDate)
		if err := validateDueDate(trimmedDue); err != nil {
			return Task{}, err
		}
	}
	if p.Labels != nil {
		var err error
		normalizedLabels, err = normalizeLabels(*p.Labels)
		if err != nil {
			return Task{}, err
		}
	}

	return s.mutate(id, func(t *Task) error {
		if p.Title != nil {
			t.Title = trimmedTitle
		}
		if p.DueDate != nil {
			t.DueDate = trimmedDue
		}
		if p.Labels != nil {
			t.Labels = normalizedLabels
		}
		if p.Done != nil {
			t.Done = *p.Done
		}
		return nil
	})
}

func (s *Store) Remove(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.tasks {
		if t.ID == id {
			s.tasks = slices.Delete(s.tasks, i, i+1)
			return s.save()
		}
	}
	return ErrNotFound
}

func (s *Store) Reorder(ids []int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(ids) != len(s.tasks) {
		return ErrReorderLength
	}
	byID := make(map[int]Task, len(s.tasks))
	for _, t := range s.tasks {
		byID[t.ID] = t
	}
	reordered := make([]Task, 0, len(ids))
	seen := make(map[int]bool, len(ids))
	for _, id := range ids {
		t, ok := byID[id]
		if !ok || seen[id] {
			return ErrReorderUnknown
		}
		seen[id] = true
		reordered = append(reordered, t)
	}
	s.tasks = reordered
	return s.save()
}

func (s *Store) GetPublicLabels() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.publicLabels)
}

func (s *Store) IsPublic(label string) bool {
	label = strings.ToLower(strings.TrimSpace(label))
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.publicSet[label]
}

func (s *Store) HasPublicLabels() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.publicLabels) > 0
}

func (s *Store) mutatePublicLabels(fn func() error) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(); err != nil {
		return nil, err
	}
	s.rebuildPublicSet()
	if err := s.save(); err != nil {
		return nil, err
	}
	return slices.Clone(s.publicLabels), nil
}

func (s *Store) SetPublicLabels(labels []string) ([]string, error) {
	normalized, err := normalizeLabels(labels)
	if err != nil {
		return nil, err
	}
	return s.mutatePublicLabels(func() error {
		s.publicLabels = normalized
		return nil
	})
}

func (s *Store) AddPublicLabel(label string) ([]string, error) {
	l, err := validateLabel(label)
	if err != nil {
		return nil, err
	}
	return s.mutatePublicLabels(func() error {
		for _, existing := range s.publicLabels {
			if existing == l {
				return nil
			}
		}
		s.publicLabels = append(s.publicLabels, l)
		return nil
	})
}

func (s *Store) RemovePublicLabel(label string) ([]string, error) {
	l, err := validateLabel(label)
	if err != nil {
		return nil, err
	}
	return s.mutatePublicLabels(func() error {
		kept := make([]string, 0, len(s.publicLabels))
		for _, existing := range s.publicLabels {
			if existing != l {
				kept = append(kept, existing)
			}
		}
		s.publicLabels = kept
		return nil
	})
}

func (s *Store) HasAnyPublicLabel(t Task) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.publicSet) == 0 {
		return false
	}
	for _, l := range t.Labels {
		if s.publicSet[l] {
			return true
		}
	}
	return false
}
