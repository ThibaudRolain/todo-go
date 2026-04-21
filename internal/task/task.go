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

type Patch struct {
	Title   *string
	DueDate *string
	Labels  *[]string
	Done    *bool
}

type SortMode string

const (
	SortManual SortMode = "manual"
	SortByDue  SortMode = "due"
)

type Status string

const (
	StatusAll     Status = "all"
	StatusPending Status = "pending"
	StatusDone    Status = "done"
)

const DateFormat = "2006-01-02"

var (
	ErrNotFound       = errors.New("task not found")
	ErrEmptyTitle     = errors.New("title must not be empty")
	ErrReorderLength  = errors.New("reorder ids must match existing tasks")
	ErrReorderUnknown = errors.New("reorder contains unknown id")
	ErrBadDueDate     = errors.New("due date must be YYYY-MM-DD")
	ErrBadLabel       = errors.New("label must be non-empty and not contain whitespace")
	ErrBadSortMode    = errors.New("sort mode must be one of: due, manual")
	ErrBadStatus      = errors.New("status must be one of: all, pending, done")
)

func ParseSortMode(s string) (SortMode, error) {
	if s == "" {
		return SortByDue, nil
	}
	m := SortMode(s)
	if m != SortManual && m != SortByDue {
		return "", ErrBadSortMode
	}
	return m, nil
}

func ParseStatus(s string) (Status, error) {
	if s == "" {
		return StatusAll, nil
	}
	st := Status(s)
	if st != StatusAll && st != StatusPending && st != StatusDone {
		return "", ErrBadStatus
	}
	return st, nil
}
