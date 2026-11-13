//go:build windows

package main

import (
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

// startDetached starts name with no console and in a process group of its own,
// so it outlives this process and the terminal, with stdout and stderr to
// out.
func startDetached(name string, args []string, out *os.File) (*exec.Cmd, error) {
	c := exec.Command(name, args...)
	c.Stdout, c.Stderr = out, out
	c.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	return c, c.Start()
}
