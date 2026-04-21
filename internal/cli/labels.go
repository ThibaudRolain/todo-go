package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

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

func newPublicCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "public <label>",
		Short: "Mark a label as public (tasks with this label become visible to other users)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := open()
			if err != nil {
				return err
			}
			labels, err := store.AddPublicLabel(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("public labels: %s\n", strings.Join(labels, ", "))
			return nil
		},
	}
}

func newPrivateCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "private <label>",
		Short: "Make a label private again",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := open()
			if err != nil {
				return err
			}
			labels, err := store.RemovePublicLabel(args[0])
			if err != nil {
				return err
			}
			if len(labels) == 0 {
				fmt.Println("no public labels")
			} else {
				fmt.Printf("public labels: %s\n", strings.Join(labels, ", "))
			}
			return nil
		},
	}
}

func newPublicLabelsCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "public-labels",
		Short: "List this user's public labels",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := open()
			if err != nil {
				return err
			}
			labels := store.GetPublicLabels()
			if len(labels) == 0 {
				fmt.Println("(none)")
				return nil
			}
			for _, l := range labels {
				fmt.Println(l)
			}
			return nil
		},
	}
}
