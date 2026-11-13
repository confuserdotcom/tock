//go:build !unix && !windows

package main

import (
	"os"
	"os/exec"
)

// detach starts name with stdout and stderr to out and lets it run on.
func detach(name string, args []string, out *os.File) error {
	c := exec.Command(name, args...)
	c.Stdout, c.Stderr = out, out
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}
