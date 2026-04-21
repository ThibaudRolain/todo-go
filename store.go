package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Task struct {
	ID      int      `json:"id"`
	Title   string   `json:"title"`
	Done    bool     `json:"done"`
	DueDate string   `json:"due_date,omitempty"`
	Labels  []string `json:"labels,omitempty"`
}

type Store struct {
	mu     sync.Mutex
	path   string
	NextID int    `json:"next_id"`
	Tasks  []Task `json:"tasks"`
}

// NewTask is the input to Add. Makes it easy to add fields without breaking callers.
type NewTask struct {
	Title   string
	DueDate string
	Labels  []string
}

const DateFormat = "2006-01-02"

var (
	ErrNotFound       = errors.New("task not found")
	ErrEmptyTitle     = errors.New("title must not be empty")
	ErrReorderLength  = errors.New("reorder ids must match existing tasks")
	ErrReorderUnknown = errors.New("reorder contains unknown id")
	ErrBadDueDate     = errors.New("due date must be YYYY-MM-DD")
	ErrBadLabel       = errors.New("label must be non-empty and not contain whitespace")
)

type SortMode string

const (
	SortManual SortMode = "manual"
	SortByDue  SortMode = "due"
)

func validateDueDate(s string) error {
	if s == "" {
		return nil
	}
	if _, err := time.Parse(DateFormat, s); err != nil {
		return ErrBadDueDate
	}
	return nil
}

func normalizeLabels(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" {
			return nil, ErrBadLabel
		}
		if strings.ContainsAny(l, " \t\n\r") {
			return nil, ErrBadLabel
		}
		if seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	return out, nil
}

func validateLabel(label string) (string, error) {
	normalized, err := normalizeLabels([]string{label})
	if err != nil {
		return "", err
	}
	if len(normalized) == 0 {
		return "", ErrBadLabel
	}
	return normalized[0], nil
}

// defaultDataDir is the root directory under which per-user data lives.
// Override via TODO_GO_DATA env var (now pointing at a directory, not a file).
func defaultDataDir() (string, error) {
	if p := os.Getenv("TODO_GO_DATA"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".todo-go"), nil
}

// userStorePath returns the path to tasks.json for a given user.
func userStorePath(username string) (string, error) {
	base, err := defaultDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "users", username, "tasks.json"), nil
}

// migrateLegacyTasks moves ~/.todo-go/tasks.json → ~/.todo-go/users/<target>/tasks.json
// if the legacy file exists and the target doesn't. Returns whether a migration happened.
func migrateLegacyTasks(target string) (bool, error) {
	base, err := defaultDataDir()
	if err != nil {
		return false, err
	}
	legacy := filepath.Join(base, "tasks.json")
	if _, err := os.Stat(legacy); err != nil {
		return false, nil
	}
	newPath, err := userStorePath(target)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(newPath); err == nil {
		return false, nil // already exists, don't overwrite
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return false, err
	}
	if err := os.Rename(legacy, newPath); err != nil {
		return false, err
	}
	return true, nil
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

// OpenUserStore opens the store for a given username, creating the dir if needed.
func OpenUserStore(username string) (*Store, error) {
	path, err := userStorePath(username)
	if err != nil {
		return nil, err
	}
	return OpenStore(path)
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

func SortTasks(tasks []Task, mode SortMode) {
	if mode != SortByDue {
		return
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		a, b := tasks[i], tasks[j]
		if a.Done != b.Done {
			return !a.Done
		}
		aHas, bHas := a.DueDate != "", b.DueDate != ""
		if aHas != bHas {
			return aHas
		}
		if aHas && a.DueDate != b.DueDate {
			return a.DueDate < b.DueDate
		}
		return a.ID < b.ID
	})
}

func IsOverdue(t Task, today time.Time) bool {
	if t.Done || t.DueDate == "" {
		return false
	}
	due, err := time.Parse(DateFormat, t.DueDate)
	if err != nil {
		return false
	}
	todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	return due.Before(todayStart)
}

func HasLabel(t Task, label string) bool {
	label = strings.ToLower(strings.TrimSpace(label))
	for _, l := range t.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// StoreManager caches per-user Stores so concurrent requests from the same user
// share a single mutex.
type StoreManager struct {
	mu     sync.Mutex
	stores map[string]*Store
}

func NewStoreManager() *StoreManager {
	return &StoreManager{stores: make(map[string]*Store)}
}

func (sm *StoreManager) ForUser(username string) (*Store, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.stores[username]; ok {
		return s, nil
	}
	s, err := OpenUserStore(username)
	if err != nil {
		return nil, err
	}
	sm.stores[username] = s
	return s, nil
}
