package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

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
