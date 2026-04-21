package task

import "errors"

type Task struct {
	ID      int      `json:"id"`
	Title   string   `json:"title"`
	Done    bool     `json:"done"`
	DueDate string   `json:"due_date,omitempty"`
	Labels  []string `json:"labels,omitempty"`
}

type NewTask struct {
	Title   string
	DueDate string
	Labels  []string
}

type SortMode string

const (
	SortManual SortMode = "manual"
	SortByDue  SortMode = "due"
)

const DateFormat = "2006-01-02"

var (
	ErrNotFound       = errors.New("task not found")
	ErrEmptyTitle     = errors.New("title must not be empty")
	ErrReorderLength  = errors.New("reorder ids must match existing tasks")
	ErrReorderUnknown = errors.New("reorder contains unknown id")
	ErrBadDueDate     = errors.New("due date must be YYYY-MM-DD")
	ErrBadLabel       = errors.New("label must be non-empty and not contain whitespace")
)
