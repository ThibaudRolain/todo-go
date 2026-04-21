package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"todo-go/internal/session"
	"todo-go/internal/task"
)

type TaskView struct {
	task.Task
	Owner  string `json:"owner"`
	Public bool   `json:"public"`
}

func aggregatedTasks(deps Deps, currentUser string) ([]TaskView, error) {
	out := []TaskView{}

	me, err := deps.Stores.ForUser(currentUser)
	if err != nil {
		return nil, err
	}
	myPublic := make(map[string]bool)
	for _, l := range me.GetPublicLabels() {
		myPublic[l] = true
	}
	for _, t := range me.List() {
		isPublic := false
		for _, l := range t.Labels {
			if myPublic[l] {
				isPublic = true
				break
			}
		}
		out = append(out, TaskView{Task: t, Owner: currentUser, Public: isPublic})
	}

	for _, other := range deps.Users.Usernames() {
		if other == currentUser {
			continue
		}
		s, err := deps.Stores.ForUser(other)
		if err != nil {
			continue
		}
		otherPublic := make(map[string]bool)
		for _, l := range s.GetPublicLabels() {
			otherPublic[l] = true
		}
		if len(otherPublic) == 0 {
			continue
		}
		for _, t := range s.List() {
			shared := false
			for _, l := range t.Labels {
				if otherPublic[l] {
					shared = true
					break
				}
			}
			if shared {
				out = append(out, TaskView{Task: t, Owner: other, Public: true})
			}
		}
	}
	return out, nil
}

func filterViewsByDone(views []TaskView, done bool) []TaskView {
	out := views[:0:0]
	for _, v := range views {
		if v.Done == done {
			out = append(out, v)
		}
	}
	return out
}

func filterViewsByLabel(views []TaskView, label string) []TaskView {
	out := views[:0:0]
	for _, v := range views {
		if task.HasLabel(v.Task, label) {
			out = append(out, v)
		}
	}
	return out
}

func sortViews(views []TaskView, mode task.SortMode) {
	if mode != task.SortByDue {
		return
	}
	sort.SliceStable(views, func(i, j int) bool {
		a, b := views[i], views[j]
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
		if a.Owner != b.Owner {
			return a.Owner < b.Owner
		}
		return a.ID < b.ID
	})
}

func tasksHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current := session.UsernameFromRequest(r)

		switch r.Method {
		case http.MethodGet:
			views, err := aggregatedTasks(deps, current)
			if err != nil {
				http.Error(w, "failed to aggregate tasks", http.StatusInternalServerError)
				return
			}
			switch r.URL.Query().Get("status") {
			case "pending":
				views = filterViewsByDone(views, false)
			case "done":
				views = filterViewsByDone(views, true)
			case "", "all":
			default:
				http.Error(w, "invalid status; want one of: pending, done, all", http.StatusBadRequest)
				return
			}
			if label := r.URL.Query().Get("label"); label != "" {
				views = filterViewsByLabel(views, label)
			}
			sortParam := r.URL.Query().Get("sort")
			if sortParam == "" {
				sortParam = string(task.SortByDue)
			}
			mode := task.SortMode(sortParam)
			if mode != task.SortManual && mode != task.SortByDue {
				http.Error(w, "invalid sort; want one of: due, manual", http.StatusBadRequest)
				return
			}
			sortViews(views, mode)
			writeJSON(w, http.StatusOK, views)

		case http.MethodPost:
			store, err := deps.Stores.ForUser(current)
			if err != nil {
				http.Error(w, "failed to open store", http.StatusInternalServerError)
				return
			}
			var body struct {
				Title   string   `json:"title"`
				DueDate string   `json:"due_date"`
				Labels  []string `json:"labels"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			title := strings.TrimSpace(body.Title)
			if title == "" {
				http.Error(w, "title required", http.StatusBadRequest)
				return
			}
			t, err := store.Add(task.NewTask{Title: title, DueDate: body.DueDate, Labels: body.Labels})
			if err != nil {
				writeStoreErr(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, TaskView{Task: t, Owner: current, Public: store.HasAnyPublicLabel(t)})

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func taskByIDHandler(mgr *task.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current := session.UsernameFromRequest(r)
		store, err := mgr.ForUser(current)
		if err != nil {
			http.Error(w, "failed to open store", http.StatusInternalServerError)
			return
		}
		idStr := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
		idStr = strings.TrimSuffix(idStr, "/")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodPatch:
			var body struct {
				Done    *bool     `json:"done"`
				Title   *string   `json:"title"`
				DueDate *string   `json:"due_date"`
				Labels  *[]string `json:"labels"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			if body.Done == nil && body.Title == nil && body.DueDate == nil && body.Labels == nil {
				http.Error(w, "no fields to update", http.StatusBadRequest)
				return
			}
			var updated task.Task
			if body.Title != nil {
				title := strings.TrimSpace(*body.Title)
				if title == "" {
					http.Error(w, "title must not be empty", http.StatusBadRequest)
					return
				}
				updated, err = store.SetTitle(id, title)
				if err != nil {
					writeStoreErr(w, err)
					return
				}
			}
			if body.DueDate != nil {
				updated, err = store.SetDue(id, strings.TrimSpace(*body.DueDate))
				if err != nil {
					writeStoreErr(w, err)
					return
				}
			}
			if body.Labels != nil {
				updated, err = store.SetLabels(id, *body.Labels)
				if err != nil {
					writeStoreErr(w, err)
					return
				}
			}
			if body.Done != nil {
				updated, err = store.SetDone(id, *body.Done)
				if err != nil {
					writeStoreErr(w, err)
					return
				}
			}
			writeJSON(w, http.StatusOK, TaskView{Task: updated, Owner: current, Public: store.HasAnyPublicLabel(updated)})

		case http.MethodDelete:
			if err := store.Remove(id); err != nil {
				writeStoreErr(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func reorderHandler(mgr *task.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current := session.UsernameFromRequest(r)
		store, err := mgr.ForUser(current)
		if err != nil {
			http.Error(w, "failed to open store", http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			IDs []int `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if err := store.Reorder(body.IDs); err != nil {
			switch {
			case errors.Is(err, task.ErrReorderLength), errors.Is(err, task.ErrReorderUnknown):
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		writeJSON(w, http.StatusOK, store.List())
	}
}
