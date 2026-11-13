//go:build unix

package main

import (
	"os"
	"syscall"
)

// privateFiles makes every file trackd creates owner-only.
func privateFiles() { syscall.Umask(0o077) }

// execInto replaces this process with the binary at path, same pid.
func execInto(path string) error { return syscall.Exec(path, os.Args, os.Environ()) }
