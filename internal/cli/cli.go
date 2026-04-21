package cli

import (
	"fmt"
	"os"

	"todo-go/internal/server"
	"todo-go/internal/session"
	"todo-go/internal/task"
	"todo-go/internal/user"

	"github.com/spf13/cobra"
)

type storeOpener func() (*task.Store, error)

func NewRootCmd() *cobra.Command {
	var username string
	root := &cobra.Command{
		Use:           "todo-go",
		Short:         "A tiny multi-user to-do list: CLI, HTTP API, and web UI",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.PersistentFlags().StringVar(&username, "user", "", `which user's tasks to act on (env: TODO_GO_USER; default: "default")`)

	open := func() (*task.Store, error) {
		u := resolveUser(username)
		return task.OpenForUser(u)
	}

	root.AddCommand(
		newAddCmd(open),
		newListCmd(open),
		newDoneCmd(open),
		newUndoneCmd(open),
		newEditCmd(open),
		newDueCmd(open),
		newUndueCmd(open),
		newTagCmd(open),
		newUntagCmd(open),
		newLabelsCmd(open),
		newPublicCmd(open),
		newPrivateCmd(open),
		newPublicLabelsCmd(open),
		newRemoveCmd(open),
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

func newServeCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server and web UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			if envAddr := os.Getenv("TODO_GO_ADDR"); envAddr != "" && !cmd.Flags().Changed("addr") {
				addr = envAddr
			}
			users, err := user.Open("")
			if err != nil {
				return fmt.Errorf("open users: %w", err)
			}
			return server.Run(server.Deps{
				Users:    users,
				Stores:   task.NewManager(),
				Sessions: session.NewManager(),
			}, addr)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "localhost:8080", "address to listen on (env: TODO_GO_ADDR)")
	return cmd
}
