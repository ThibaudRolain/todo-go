package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	var user string
	root := &cobra.Command{
		Use:           "todo-go",
		Short:         "A tiny multi-user to-do list: CLI, HTTP API, and web UI",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.PersistentFlags().StringVar(&user, "user", "", `which user's tasks to act on (env: TODO_GO_USER; default: "default")`)

	openStore := func() (*Store, error) {
		u := resolveUser(user)
		return OpenUserStore(u)
	}

	root.AddCommand(
		newAddCmd(openStore),
		newListCmd(openStore),
		newDoneCmd(openStore),
		newUndoneCmd(openStore),
		newEditCmd(openStore),
		newDueCmd(openStore),
		newUndueCmd(openStore),
		newTagCmd(openStore),
		newUntagCmd(openStore),
		newLabelsCmd(openStore),
		newRemoveCmd(openStore),
		newServeCmd(),
	)
	return root
}

func resolveUser(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv("TODO_GO_USER"); env != "" {
		return env
	}
	return "default"
}

type storeOpener func() (*Store, error)

func newAddCmd(open storeOpener) *cobra.Command {
	var due string
	var labels []string
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a new task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := open()
			if err != nil {
				return err
			}
			t, err := store.Add(NewTask{Title: args[0], DueDate: due, Labels: labels})
			if err != nil {
				return err
			}
			fmt.Println(formatTask(t))
			return nil
		},
	}
	cmd.Flags().StringVar(&due, "due", "", "due date (YYYY-MM-DD)")
	cmd.Flags().StringSliceVarP(&labels, "label", "l", nil, "label to attach (repeatable)")
	return cmd
}

func newListCmd(open storeOpener) *cobra.Command {
	var pending, done bool
	var sortMode string
	var label string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := open()
			if err != nil {
				return err
			}
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
			if label != "" {
				tasks = filterByLabel(tasks, label)
			}
			SortTasks(tasks, mode)
			if len(tasks) == 0 {
				fmt.Println("(no tasks)")
				return nil
			}
			for _, t := range tasks {
				fmt.Println(formatTaskLine(t, time.Now()))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&pending, "pending", false, "only show tasks that are not done")
	cmd.Flags().BoolVar(&done, "done", false, "only show tasks that are done")
	cmd.Flags().StringVar(&sortMode, "sort", string(SortByDue), "sort order: due, manual")
	cmd.Flags().StringVarP(&label, "label", "l", "", "only show tasks with this label")
	return cmd
}

func newDoneCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "done <id>",
		Short: "Mark a task as done",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := open()
			if err != nil {
				return err
			}
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

func newUndoneCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "undone <id>",
		Short: "Mark a task as not done",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := open()
			if err != nil {
				return err
			}
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

func newEditCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "edit <id> <new-title>",
		Short: "Edit a task's title",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := open()
			if err != nil {
				return err
			}
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

func newDueCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "due <id> <date>",
		Short: "Set or change a task's due date (YYYY-MM-DD)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := open()
			if err != nil {
				return err
			}
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

func newUndueCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "undue <id>",
		Short: "Clear a task's due date",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := open()
			if err != nil {
				return err
			}
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

func newTagCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "tag <id> <label>",
		Short: "Add a label to a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := open()
			if err != nil {
				return err
			}
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			t, err := store.AddLabel(id, args[1])
			if err != nil {
				return err
			}
			fmt.Printf("tagged #%d: %s\n", t.ID, strings.Join(t.Labels, ", "))
			return nil
		},
	}
}

func newUntagCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "untag <id> <label>",
		Short: "Remove a label from a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := open()
			if err != nil {
				return err
			}
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			t, err := store.RemoveLabel(id, args[1])
			if err != nil {
				return err
			}
			if len(t.Labels) == 0 {
				fmt.Printf("removed label from #%d (no labels remain)\n", t.ID)
			} else {
				fmt.Printf("untagged #%d: %s\n", t.ID, strings.Join(t.Labels, ", "))
			}
			return nil
		},
	}
}

func newLabelsCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "labels",
		Short: "List all labels in use",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := open()
			if err != nil {
				return err
			}
			labels := store.Labels()
			if len(labels) == 0 {
				fmt.Println("(no labels)")
				return nil
			}
			for _, l := range labels {
				fmt.Println(l)
			}
			return nil
		},
	}
}

func newRemoveCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <id>",
		Aliases: []string{"rm"},
		Short:   "Remove a task",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := open()
			if err != nil {
				return err
			}
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

func newServeCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server and web UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			if envAddr := os.Getenv("TODO_GO_ADDR"); envAddr != "" && !cmd.Flags().Changed("addr") {
				addr = envAddr
			}
			users, err := OpenUsers("")
			if err != nil {
				return fmt.Errorf("open users: %w", err)
			}
			deps := serverDeps{
				Users:    users,
				Stores:   NewStoreManager(),
				Sessions: NewSessionManager(),
			}
			return runServer(deps, addr)
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

func filterByLabel(tasks []Task, label string) []Task {
	out := tasks[:0:0]
	for _, t := range tasks {
		if HasLabel(t, label) {
			out = append(out, t)
		}
	}
	return out
}

func formatTask(t Task) string {
	parts := []string{fmt.Sprintf("added #%d: %s", t.ID, t.Title)}
	if t.DueDate != "" {
		parts = append(parts, fmt.Sprintf("due %s", t.DueDate))
	}
	if len(t.Labels) > 0 {
		parts = append(parts, fmt.Sprintf("labels: %s", strings.Join(t.Labels, ", ")))
	}
	return strings.Join(parts, " · ")
}

func formatTaskLine(t Task, today time.Time) string {
	status := "[ ]"
	if t.Done {
		status = "[x]"
	}
	line := fmt.Sprintf("%s %d. %s", status, t.ID, t.Title)

	suffix := []string{}
	switch {
	case IsOverdue(t, today):
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
