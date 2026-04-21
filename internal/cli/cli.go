package cli

import (
	"fmt"
	"os"
	"strconv"

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
		return task.OpenForUser(envOrDefault(username, "TODO_GO_USER", "default"))
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

func envOrDefault(flagValue, envVar, fallback string) string {
	if flagValue != "" {
		return flagValue
	}
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return fallback
}

func parseID(arg string) (int, error) {
	id, err := strconv.Atoi(arg)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q", arg)
	}
	return id, nil
}

func withStore(open storeOpener, fn func(*task.Store) error) error {
	store, err := open()
	if err != nil {
		return err
	}
	return fn(store)
}

func withStoreAndID(open storeOpener, arg string, fn func(*task.Store, int) error) error {
	id, err := parseID(arg)
	if err != nil {
		return err
	}
	return withStore(open, func(s *task.Store) error { return fn(s, id) })
}

func newServeCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server and web UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("addr") {
				addr = envOrDefault("", "TODO_GO_ADDR", addr)
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
