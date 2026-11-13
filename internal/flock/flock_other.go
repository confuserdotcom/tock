//go:build !unix && !windows

package flock

import "errors"

func lock(File, bool, bool) error { return errors.New("flock: unsupported platform") }

func alive(int) bool { return true }
