//go:build unix

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// startDetached starts name in a session of its own, so it outlives this process
// and the terminal, with stdout and stderr to out.
func startDetached(name string, args []string, out *os.File) (*exec.Cmd, error) {
	c := exec.Command(name, args...)
	c.Stdout, c.Stderr = out, out
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return c, c.Start()
}
