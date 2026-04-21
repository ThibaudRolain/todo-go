package task

func FilterByDone(tasks []Task, done bool) []Task {
	out := tasks[:0:0]
	for _, t := range tasks {
		if t.Done == done {
			out = append(out, t)
		}
	}
	return out
}

func FilterByLabel(tasks []Task, label string) []Task {
	out := tasks[:0:0]
	for _, t := range tasks {
		if HasLabel(t, label) {
			out = append(out, t)
		}
	}
	return out
}
