package cli

import (
	"errors"
	"fmt"
	"time"

	"todo-go/internal/task"

	"github.com/spf13/cobra"
)

func newAddCmd(open storeOpener) *cobra.Command {
	var due string
	var labels []string
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a new task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(open, func(s *task.Store) error {
				t, err := s.Add(task.NewTask{Title: args[0], DueDate: due, Labels: labels})
				if err != nil {
					return err
				}
				fmt.Println(formatTask(t))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&due, "due", "", "due date (YYYY-MM-DD)")
	cmd.Flags().StringSliceVarP(&labels, "label", "l", nil, "label to attach (repeatable)")
	return cmd
}

func newListCmd(open storeOpener) *cobra.Command {
	var pending, done bool
	var sortFlag string
	var label string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pending && done {
				return errors.New("--pending and --done are mutually exclusive")
			}
			mode, err := task.ParseSortMode(sortFlag)
			if err != nil {
				return err
			}
			return withStore(open, func(s *task.Store) error {
				tasks := s.List()
				switch {
				case pending:
					tasks = task.FilterByDone(tasks, false)
				case done:
					tasks = task.FilterByDone(tasks, true)
				}
				if label != "" {
					tasks = task.FilterByLabel(tasks, label)
				}
				task.SortTasks(tasks, mode)
				if len(tasks) == 0 {
					fmt.Println("(no tasks)")
					return nil
				}
				for _, t := range tasks {
					fmt.Println(formatTaskLine(t, time.Now()))
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&pending, "pending", false, "only show tasks that are not done")
	cmd.Flags().BoolVar(&done, "done", false, "only show tasks that are done")
	cmd.Flags().StringVar(&sortFlag, "sort", string(task.SortByDue), "sort order: due, manual")
	cmd.Flags().StringVarP(&label, "label", "l", "", "only show tasks with this label")
	return cmd
}

func newDoneCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "done <id>",
		Short: "Mark a task as done",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStoreAndID(open, args[0], func(s *task.Store, id int) error {
				if _, err := s.SetDone(id, true); err != nil {
					return err
				}
				fmt.Printf("marked #%d done\n", id)
				return nil
			})
		},
	}
}

func newUndoneCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "undone <id>",
		Short: "Mark a task as not done",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStoreAndID(open, args[0], func(s *task.Store, id int) error {
				if _, err := s.SetDone(id, false); err != nil {
					return err
				}
				fmt.Printf("unmarked #%d\n", id)
				return nil
			})
		},
	}
}

func newEditCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "edit <id> <new-title>",
		Short: "Edit a task's title",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStoreAndID(open, args[0], func(s *task.Store, id int) error {
				t, err := s.SetTitle(id, args[1])
				if err != nil {
					return err
				}
				fmt.Printf("updated #%d: %s\n", t.ID, t.Title)
				return nil
			})
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
			return withStoreAndID(open, args[0], func(s *task.Store, id int) error {
				if err := s.Remove(id); err != nil {
					return err
				}
				fmt.Printf("removed #%d\n", id)
				return nil
			})
		},
	}
}
