package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Task struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

type Store struct {
	mu     sync.Mutex
	path   string
	NextID int    `json:"next_id"`
	Tasks  []Task `json:"tasks"`
}

var (
	ErrNotFound      = errors.New("task not found")
	ErrEmptyTitle    = errors.New("title must not be empty")
	ErrReorderLength = errors.New("reorder ids must match existing tasks")
	ErrReorderUnknown = errors.New("reorder contains unknown id")
)

func defaultStorePath() (string, error) {
	if p := os.Getenv("TODO_GO_DATA"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".todo-go", "tasks.json"), nil
}

func OpenStore(path string) (*Store, error) {
	if path == "" {
		p, err := defaultStorePath()
		if err != nil {
			return nil, err
		}
		path = p
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

func (s *Store) Add(title string) (Task, error) {
	if title == "" {
		return Task{}, ErrEmptyTitle
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := Task{ID: s.NextID, Title: title}
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
