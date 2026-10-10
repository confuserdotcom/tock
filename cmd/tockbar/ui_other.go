//go:build !darwin || !cgo

package main

import (
	"fmt"
	"os"
)

func runUI(*runner) {
	fmt.Fprintln(os.Stderr, "tockbar: the menu bar app needs macOS and cgo")
	os.Exit(1)
}
