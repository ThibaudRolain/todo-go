package cli

import (
	"fmt"

	"todo-go/internal/task"

	"github.com/spf13/cobra"
)

func newDueCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "due <id> <date>",
		Short: "Set or change a task's due date (YYYY-MM-DD)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStoreAndID(open, args[0], func(s *task.Store, id int) error {
				t, err := s.SetDue(id, args[1])
				if err != nil {
					return err
				}
				fmt.Printf("set #%d due to %s\n", t.ID, t.DueDate)
				return nil
			})
		},
	}
}

func newUndueCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "undue <id>",
		Short: "Clear a task's due date",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStoreAndID(open, args[0], func(s *task.Store, id int) error {
				if _, err := s.SetDue(id, ""); err != nil {
					return err
				}
				fmt.Printf("cleared due date on #%d\n", id)
				return nil
			})
		},
	}
}
