package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"todo-go/internal/task"
)

func formatTask(t task.Task) string {
	parts := []string{fmt.Sprintf("added #%d: %s", t.ID, t.Title)}
	if t.DueDate != "" {
		parts = append(parts, fmt.Sprintf("due %s", t.DueDate))
	}
	if len(t.Labels) > 0 {
		parts = append(parts, fmt.Sprintf("labels: %s", strings.Join(t.Labels, ", ")))
	}
	return strings.Join(parts, " · ")
}

func formatTaskLine(t task.Task, today time.Time) string {
	status := "[ ]"
	if t.Done {
		status = "[x]"
	}
	line := fmt.Sprintf("%s %d. %s", status, t.ID, t.Title)

	suffix := []string{}
	switch {
	case task.IsOverdue(t, today):
		suffix = append(suffix, fmt.Sprintf("OVERDUE (%s)", t.DueDate))
	case t.DueDate != "":
		suffix = append(suffix, fmt.Sprintf("due %s", t.DueDate))
	}
	if len(t.Labels) > 0 {
		tagStr := make([]string, len(t.Labels))
		copy(tagStr, t.Labels)
		sort.Strings(tagStr)
		for i, l := range tagStr {
			tagStr[i] = "#" + l
		}
		suffix = append(suffix, strings.Join(tagStr, " "))
	}
	if len(suffix) > 0 {
		line += " — " + strings.Join(suffix, " · ")
	}
	return line
}
