package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	store, err := OpenStore("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "load error:", err)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "add":
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "usage: todo-go add <title>")
			os.Exit(1)
		}
		t, err := store.Add(args[0])
		exitIfErr(err)
		fmt.Printf("added #%d: %s\n", t.ID, t.Title)

	case "list":
		tasks := store.List()
		if len(tasks) == 0 {
			fmt.Println("(no tasks)")
			return
		}
		for _, t := range tasks {
			status := "[ ]"
			if t.Done {
				status = "[x]"
			}
			fmt.Printf("%s %d. %s\n", status, t.ID, t.Title)
		}

	case "done":
		id := mustParseID(args)
		_, err := store.SetDone(id, true)
		if errors.Is(err, ErrNotFound) {
			fmt.Fprintf(os.Stderr, "no task with id %d\n", id)
			os.Exit(1)
		}
		exitIfErr(err)
		fmt.Printf("marked #%d done\n", id)

	case "undone":
		id := mustParseID(args)
		_, err := store.SetDone(id, false)
		if errors.Is(err, ErrNotFound) {
			fmt.Fprintf(os.Stderr, "no task with id %d\n", id)
			os.Exit(1)
		}
		exitIfErr(err)
		fmt.Printf("unmarked #%d\n", id)

	case "remove", "rm":
		id := mustParseID(args)
		err := store.Remove(id)
		if errors.Is(err, ErrNotFound) {
			fmt.Fprintf(os.Stderr, "no task with id %d\n", id)
			os.Exit(1)
		}
		exitIfErr(err)
		fmt.Printf("removed #%d\n", id)

	case "serve":
		addr := "localhost:8080"
		if len(args) >= 1 {
			addr = args[0]
		}
		if err := runServer(store, addr); err != nil {
			fmt.Fprintln(os.Stderr, "server error:", err)
			os.Exit(1)
		}

	case "help", "-h", "--help":
		usage()

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`todo-go — a tiny to-do list

Commands:
  add <title>      add a new task
  list             list all tasks
  done <id>        mark a task done
  undone <id>      mark a task not done
  remove <id>      remove a task (alias: rm)
  serve [addr]     start the web UI (default: localhost:8080)
  help             show this message`)
}

func mustParseID(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "missing id")
		os.Exit(1)
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid id %q\n", args[0])
		os.Exit(1)
	}
	return id
}

func exitIfErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
