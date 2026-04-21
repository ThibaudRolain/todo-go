package task

func FilterByDone[T interface{ GetDone() bool }](items []T, done bool) []T {
	out := items[:0:0]
	for _, it := range items {
		if it.GetDone() == done {
			out = append(out, it)
		}
	}
	return out
}

func FilterByLabel[T interface{ GetLabels() []string }](items []T, label string) []T {
	out := items[:0:0]
	for _, it := range items {
		if hasLabel(it.GetLabels(), label) {
			out = append(out, it)
		}
	}
	return out
}

func (t Task) GetDone() bool       { return t.Done }
func (t Task) GetLabels() []string { return t.Labels }

func hasLabel(labels []string, label string) bool {
	label = normalizeForCompare(label)
	for _, l := range labels {
		if l == label {
			return true
		}
	}
	return false
}

func normalizeForCompare(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(out)
}
