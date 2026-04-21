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

func (v TaskView) GetDone() bool       { return v.Done }
func (v TaskView) GetLabels() []string { return v.Labels }

func aggregatedTasks(deps Deps, currentUser string) ([]TaskView, error) {
	out := []TaskView{}

	me, err := deps.Stores.ForUser(currentUser)
	if err != nil {
		return nil, err
	}
	for _, t := range me.List() {
		out = append(out, TaskView{Task: t, Owner: currentUser, Public: me.HasAnyPublicLabel(t)})
	}

	for _, other := range deps.Users.Usernames() {
		if other == currentUser {
			continue
		}
		s, err := deps.Stores.ForUser(other)
		if err != nil {
			continue
		}
		if !s.HasPublicLabels() {
			continue
		}
		for _, t := range s.List() {
			if s.HasAnyPublicLabel(t) {
				out = append(out, TaskView{Task: t, Owner: other, Public: true})
			}
		}
	}
	return out, nil
}

func sortViews(views []TaskView, mode task.SortMode) {
	if mode != task.SortByDue {
		return
	}
	sort.SliceStable(views, func(i, j int) bool {
		a, b := views[i].Task, views[j].Task
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
		if views[i].Owner != views[j].Owner {
			return views[i].Owner < views[j].Owner
		}
		return a.ID < b.ID
	})
}

func tasksHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current := session.UsernameFromRequest(r)

		switch r.Method {
		case http.MethodGet:
			status, err := task.ParseStatus(r.URL.Query().Get("status"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mode, err := task.ParseSortMode(r.URL.Query().Get("sort"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			views, err := aggregatedTasks(deps, current)
			if err != nil {
				http.Error(w, "failed to aggregate tasks", http.StatusInternalServerError)
				return
			}
			switch status {
			case task.StatusPending:
				views = task.FilterByDone(views, false)
			case task.StatusDone:
				views = task.FilterByDone(views, true)
			}
			if label := r.URL.Query().Get("label"); label != "" {
				views = task.FilterByLabel(views, label)
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
			updated, err := store.Update(id, task.Patch{
				Title:   body.Title,
				DueDate: body.DueDate,
				Labels:  body.Labels,
				Done:    body.Done,
			})
			if err != nil {
				writeStoreErr(w, err)
				return
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
