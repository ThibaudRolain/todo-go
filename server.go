package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
)

//go:embed web
var webFS embed.FS

func runServer(store *Store, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tasks", apiTasksHandler(store))
	mux.HandleFunc("/api/tasks/", apiTaskByIDHandler(store))
	mux.HandleFunc("/api/reorder", apiReorderHandler(store))

	webRoot, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(webRoot)))

	fmt.Printf("todo-go serving on http://%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func apiTasksHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			tasks := store.List()
			switch r.URL.Query().Get("status") {
			case "pending":
				tasks = filterByDone(tasks, false)
			case "done":
				tasks = filterByDone(tasks, true)
			case "", "all":
				// no filter
			default:
				http.Error(w, "invalid status; want one of: pending, done, all", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, tasks)
		case http.MethodPost:
			var body struct {
				Title string `json:"title"`
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
			t, err := store.Add(title)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusCreated, t)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func apiTaskByIDHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
				Done  *bool   `json:"done"`
				Title *string `json:"title"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			if body.Done == nil && body.Title == nil {
				http.Error(w, "no fields to update", http.StatusBadRequest)
				return
			}

			var updated Task
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
			if body.Done != nil {
				updated, err = store.SetDone(id, *body.Done)
				if err != nil {
					writeStoreErr(w, err)
					return
				}
			}
			writeJSON(w, http.StatusOK, updated)

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

func apiReorderHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
			case errors.Is(err, ErrReorderLength), errors.Is(err, ErrReorderUnknown):
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		writeJSON(w, http.StatusOK, store.List())
	}
}

func writeStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrEmptyTitle):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
