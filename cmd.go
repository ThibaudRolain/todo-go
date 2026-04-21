package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

func newRootCmd(store *Store) *cobra.Command {
	root := &cobra.Command{
		Use:           "todo-go",
		Short:         "A tiny to-do list: CLI, HTTP API, and web UI",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.AddCommand(
		newAddCmd(store),
		newListCmd(store),
		newDoneCmd(store),
		newUndoneCmd(store),
		newEditCmd(store),
		newDueCmd(store),
		newUndueCmd(store),
		newRemoveCmd(store),
		newServeCmd(store),
	)
	return root
}

func newAddCmd(store *Store) *cobra.Command {
	var due string
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a new task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := store.Add(args[0], due)
			if err != nil {
				return err
			}
			if t.DueDate != "" {
				fmt.Printf("added #%d: %s (due %s)\n", t.ID, t.Title, t.DueDate)
			} else {
				fmt.Printf("added #%d: %s\n", t.ID, t.Title)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&due, "due", "", "due date (YYYY-MM-DD)")
	return cmd
}

func newListCmd(store *Store) *cobra.Command {
	var pending, done bool
	var sortMode string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pending && done {
				return errors.New("--pending and --done are mutually exclusive")
			}
			mode := SortMode(sortMode)
			if mode != SortManual && mode != SortByDue {
				return fmt.Errorf("invalid --sort %q (want: manual, due)", sortMode)
			}
			tasks := store.List()
			switch {
			case pending:
				tasks = filterByDone(tasks, false)
			case done:
				tasks = filterByDone(tasks, true)
			}
			SortTasks(tasks, mode)
			if len(tasks) == 0 {
				fmt.Println("(no tasks)")
				return nil
			}
			today := time.Now()
			for _, t := range tasks {
				status := "[ ]"
				if t.Done {
					status = "[x]"
				}
				line := fmt.Sprintf("%s %d. %s", status, t.ID, t.Title)
				switch {
				case IsOverdue(t, today):
					line += fmt.Sprintf(" — OVERDUE (%s)", t.DueDate)
				case t.DueDate != "":
					line += fmt.Sprintf(" — due %s", t.DueDate)
				}
				fmt.Println(line)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&pending, "pending", false, "only show tasks that are not done")
	cmd.Flags().BoolVar(&done, "done", false, "only show tasks that are done")
	cmd.Flags().StringVar(&sortMode, "sort", string(SortByDue), "sort order: due, manual")
	return cmd
}

func newDoneCmd(store *Store) *cobra.Command {
	return &cobra.Command{
		Use:   "done <id>",
		Short: "Mark a task as done",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			if _, err := store.SetDone(id, true); err != nil {
				return err
			}
			fmt.Printf("marked #%d done\n", id)
			return nil
		},
	}
}

func newUndoneCmd(store *Store) *cobra.Command {
	return &cobra.Command{
		Use:   "undone <id>",
		Short: "Mark a task as not done",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			if _, err := store.SetDone(id, false); err != nil {
				return err
			}
			fmt.Printf("unmarked #%d\n", id)
			return nil
		},
	}
}

func newEditCmd(store *Store) *cobra.Command {
	return &cobra.Command{
		Use:   "edit <id> <new-title>",
		Short: "Edit a task's title",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			t, err := store.SetTitle(id, args[1])
			if err != nil {
				return err
			}
			fmt.Printf("updated #%d: %s\n", t.ID, t.Title)
			return nil
		},
	}
}

func newDueCmd(store *Store) *cobra.Command {
	return &cobra.Command{
		Use:   "due <id> <date>",
		Short: "Set or change a task's due date (YYYY-MM-DD)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			t, err := store.SetDue(id, args[1])
			if err != nil {
				return err
			}
			fmt.Printf("set #%d due to %s\n", t.ID, t.DueDate)
			return nil
		},
	}
}

func newUndueCmd(store *Store) *cobra.Command {
	return &cobra.Command{
		Use:   "undue <id>",
		Short: "Clear a task's due date",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			if _, err := store.SetDue(id, ""); err != nil {
				return err
			}
			fmt.Printf("cleared due date on #%d\n", id)
			return nil
		},
	}
}

func newRemoveCmd(store *Store) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <id>",
		Aliases: []string{"rm"},
		Short:   "Remove a task",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			if err := store.Remove(id); err != nil {
				return err
			}
			fmt.Printf("removed #%d\n", id)
			return nil
		},
	}
}

func newServeCmd(store *Store) *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server and web UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			if envAddr := os.Getenv("TODO_GO_ADDR"); envAddr != "" && !cmd.Flags().Changed("addr") {
				addr = envAddr
			}
			return runServer(store, addr)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "localhost:8080", "address to listen on (env: TODO_GO_ADDR)")
	return cmd
}

func filterByDone(tasks []Task, done bool) []Task {
	out := tasks[:0:0]
	for _, t := range tasks {
		if t.Done == done {
			out = append(out, t)
		}
	}
	return out
}
