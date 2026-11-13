//go:build !unix && !windows

package main

import (
	"os"
	"os/exec"
)

// startDetached starts name with stdout and stderr to out and lets it run on.
func startDetached(name string, args []string, out *os.File) (*exec.Cmd, error) {
	c := exec.Command(name, args...)
	c.Stdout, c.Stderr = out, out
	return c, c.Start()
}
