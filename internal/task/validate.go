package task

import (
	"strings"
	"time"
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

