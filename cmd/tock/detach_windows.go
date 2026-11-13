//go:build windows

package main

import (
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

// detach starts name with no console and in a process group of its own,
// so it outlives this process and the terminal, with stdout and stderr to
// out.
func detach(name string, args []string, out *os.File) error {
	c := exec.Command(name, args...)
	c.Stdout, c.Stderr = out, out
	c.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}
