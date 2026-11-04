//go:build unix

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// detach starts name in a session of its own, so it outlives this process
// and the terminal, with stdout and stderr to out.
func detach(name string, args []string, out *os.File) error {
	c := exec.Command(name, args...)
	c.Stdout, c.Stderr = out, out
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}
