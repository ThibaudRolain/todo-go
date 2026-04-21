package cli

import (
	"fmt"
	"strings"

	"todo-go/internal/task"

	"github.com/spf13/cobra"
)

func newTagCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "tag <id> <label>",
		Short: "Add a label to a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStoreAndID(open, args[0], func(s *task.Store, id int) error {
				t, err := s.AddLabel(id, args[1])
				if err != nil {
					return err
				}
				fmt.Printf("tagged #%d: %s\n", t.ID, strings.Join(t.Labels, ", "))
				return nil
			})
		},
	}
}

func newUntagCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "untag <id> <label>",
		Short: "Remove a label from a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStoreAndID(open, args[0], func(s *task.Store, id int) error {
				t, err := s.RemoveLabel(id, args[1])
				if err != nil {
					return err
				}
				if len(t.Labels) == 0 {
					fmt.Printf("removed label from #%d (no labels remain)\n", t.ID)
				} else {
					fmt.Printf("untagged #%d: %s\n", t.ID, strings.Join(t.Labels, ", "))
				}
				return nil
			})
		},
	}
}

func newLabelsCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "labels",
		Short: "List all labels in use",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(open, func(s *task.Store) error {
				labels := s.Labels()
				if len(labels) == 0 {
					fmt.Println("(no labels)")
					return nil
				}
				for _, l := range labels {
					fmt.Println(l)
				}
				return nil
			})
		},
	}
}

func newPublicCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "public <label>",
		Short: "Mark a label as public (tasks with this label become visible to other users)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(open, func(s *task.Store) error {
				labels, err := s.AddPublicLabel(args[0])
				if err != nil {
					return err
				}
				fmt.Printf("public labels: %s\n", strings.Join(labels, ", "))
				return nil
			})
		},
	}
}

func newPrivateCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "private <label>",
		Short: "Make a label private again",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(open, func(s *task.Store) error {
				labels, err := s.RemovePublicLabel(args[0])
				if err != nil {
					return err
				}
				if len(labels) == 0 {
					fmt.Println("no public labels")
				} else {
					fmt.Printf("public labels: %s\n", strings.Join(labels, ", "))
				}
				return nil
			})
		},
	}
}

func newPublicLabelsCmd(open storeOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "public-labels",
		Short: "List this user's public labels",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(open, func(s *task.Store) error {
				labels := s.GetPublicLabels()
				if len(labels) == 0 {
					fmt.Println("(none)")
					return nil
				}
				for _, l := range labels {
					fmt.Println(l)
				}
				return nil
			})
		},
	}
}
