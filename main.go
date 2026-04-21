package main

import (
	"fmt"
	"os"
)

func main() {
	store, err := OpenStore("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "load error:", err)
		os.Exit(1)
	}
	if err := newRootCmd(store).Execute(); err != nil {
		os.Exit(1)
	}
}
