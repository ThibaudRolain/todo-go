package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Store struct {
	mu           sync.Mutex
	path         string
	NextID       int      `json:"next_id"`
	Tasks        []Task   `json:"tasks"`
	PublicLabels []string `json:"public_labels,omitempty"`
}

func OpenStore(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("OpenStore: empty path")
	}
	s := &Store{path: path, NextID: 1, Tasks: []Task{}}
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
	if s.NextID == 0 {
		s.NextID = 1
	}
	if s.Tasks == nil {
		s.Tasks = []Task{}
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
	out := make([]Task, len(s.Tasks))
	copy(out, s.Tasks)
	return out
}

func (s *Store) Labels() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]bool)
	for _, t := range s.Tasks {
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
	t := Task{ID: s.NextID, Title: title, DueDate: n.DueDate, Labels: labels}
	s.NextID++
	s.Tasks = append(s.Tasks, t)
	if err := s.save(); err != nil {
		return Task{}, err
	}
	return t, nil
}

func (s *Store) SetDone(id int, done bool) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Tasks {
		if s.Tasks[i].ID == id {
			s.Tasks[i].Done = done
			if err := s.save(); err != nil {
				return Task{}, err
			}
			return s.Tasks[i], nil
		}
	}
	return Task{}, ErrNotFound
}

func (s *Store) SetTitle(id int, title string) (Task, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Task{}, ErrEmptyTitle
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Tasks {
		if s.Tasks[i].ID == id {
			s.Tasks[i].Title = title
			if err := s.save(); err != nil {
				return Task{}, err
			}
			return s.Tasks[i], nil
		}
	}
	return Task{}, ErrNotFound
}

func (s *Store) SetDue(id int, dueDate string) (Task, error) {
	if err := validateDueDate(dueDate); err != nil {
		return Task{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Tasks {
		if s.Tasks[i].ID == id {
			s.Tasks[i].DueDate = dueDate
			if err := s.save(); err != nil {
				return Task{}, err
			}
			return s.Tasks[i], nil
		}
	}
	return Task{}, ErrNotFound
}

func (s *Store) SetLabels(id int, labels []string) (Task, error) {
	normalized, err := normalizeLabels(labels)
	if err != nil {
		return Task{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Tasks {
		if s.Tasks[i].ID == id {
			s.Tasks[i].Labels = normalized
			if err := s.save(); err != nil {
				return Task{}, err
			}
			return s.Tasks[i], nil
		}
	}
	return Task{}, ErrNotFound
}

func (s *Store) AddLabel(id int, label string) (Task, error) {
	l, err := validateLabel(label)
	if err != nil {
		return Task{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Tasks {
		if s.Tasks[i].ID != id {
			continue
		}
		for _, existing := range s.Tasks[i].Labels {
			if existing == l {
				return s.Tasks[i], nil
			}
		}
		s.Tasks[i].Labels = append(s.Tasks[i].Labels, l)
		if err := s.save(); err != nil {
			return Task{}, err
		}
		return s.Tasks[i], nil
	}
	return Task{}, ErrNotFound
}

func (s *Store) RemoveLabel(id int, label string) (Task, error) {
	l, err := validateLabel(label)
	if err != nil {
		return Task{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Tasks {
		if s.Tasks[i].ID != id {
			continue
		}
		kept := s.Tasks[i].Labels[:0]
		for _, existing := range s.Tasks[i].Labels {
			if existing != l {
				kept = append(kept, existing)
			}
		}
		s.Tasks[i].Labels = kept
		if err := s.save(); err != nil {
			return Task{}, err
		}
		return s.Tasks[i], nil
	}
	return Task{}, ErrNotFound
}

func (s *Store) Remove(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.Tasks {
		if t.ID == id {
			s.Tasks = append(s.Tasks[:i:i], s.Tasks[i+1:]...)
			return s.save()
		}
	}
	return ErrNotFound
}

func (s *Store) Reorder(ids []int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(ids) != len(s.Tasks) {
		return ErrReorderLength
	}
	byID := make(map[int]Task, len(s.Tasks))
	for _, t := range s.Tasks {
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
	s.Tasks = reordered
	return s.save()
}

func (s *Store) GetPublicLabels() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.PublicLabels))
	copy(out, s.PublicLabels)
	return out
}

func (s *Store) IsPublic(label string) bool {
	label = strings.ToLower(strings.TrimSpace(label))
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.PublicLabels {
		if l == label {
			return true
		}
	}
	return false
}

func (s *Store) SetPublicLabels(labels []string) ([]string, error) {
	normalized, err := normalizeLabels(labels)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PublicLabels = normalized
	if err := s.save(); err != nil {
		return nil, err
	}
	out := make([]string, len(s.PublicLabels))
	copy(out, s.PublicLabels)
	return out, nil
}

func (s *Store) AddPublicLabel(label string) ([]string, error) {
	l, err := validateLabel(label)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.PublicLabels {
		if existing == l {
			out := make([]string, len(s.PublicLabels))
			copy(out, s.PublicLabels)
			return out, nil
		}
	}
	s.PublicLabels = append(s.PublicLabels, l)
	if err := s.save(); err != nil {
		return nil, err
	}
	out := make([]string, len(s.PublicLabels))
	copy(out, s.PublicLabels)
	return out, nil
}

func (s *Store) RemovePublicLabel(label string) ([]string, error) {
	l, err := validateLabel(label)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.PublicLabels[:0]
	for _, existing := range s.PublicLabels {
		if existing != l {
			kept = append(kept, existing)
		}
	}
	s.PublicLabels = kept
	if err := s.save(); err != nil {
		return nil, err
	}
	out := make([]string, len(s.PublicLabels))
	copy(out, s.PublicLabels)
	return out, nil
}

func (s *Store) HasAnyPublicLabel(t Task) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.PublicLabels) == 0 || len(t.Labels) == 0 {
		return false
	}
	public := make(map[string]bool, len(s.PublicLabels))
	for _, l := range s.PublicLabels {
		public[l] = true
	}
	for _, l := range t.Labels {
		if public[l] {
			return true
		}
	}
	return false
}
