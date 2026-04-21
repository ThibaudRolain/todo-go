package server

import (
	"encoding/json"
	"net/http"
	"sort"

	"todo-go/internal/session"
	"todo-go/internal/task"
)

func labelsHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		current := session.UsernameFromRequest(r)
		seen := map[string]bool{}
		if me, err := deps.Stores.ForUser(current); err == nil {
			for _, t := range me.List() {
				for _, l := range t.Labels {
					seen[l] = true
				}
			}
		}
		for _, other := range deps.Users.Usernames() {
			if other == current {
				continue
			}
			s, err := deps.Stores.ForUser(other)
			if err != nil {
				continue
			}
			for _, t := range s.List() {
				for _, l := range t.Labels {
					if s.IsPublic(l) {
						seen[l] = true
					}
				}
			}
		}
		labels := make([]string, 0, len(seen))
		for l := range seen {
			labels = append(labels, l)
		}
		sort.Strings(labels)
		writeJSON(w, http.StatusOK, labels)
	}
}

func publicLabelsHandler(mgr *task.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current := session.UsernameFromRequest(r)
		store, err := mgr.ForUser(current)
		if err != nil {
			http.Error(w, "failed to open store", http.StatusInternalServerError)
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, store.GetPublicLabels())

		case http.MethodPut:
			var body struct {
				Labels []string `json:"labels"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			updated, err := store.SetPublicLabels(body.Labels)
			if err != nil {
				writeStoreErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, updated)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
