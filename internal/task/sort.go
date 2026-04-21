package task

import (
	"sort"
	"strings"
	"time"
)

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
