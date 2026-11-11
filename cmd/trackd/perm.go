package main

import (
	"errors"
	"io/fs"
	"os"
)

// tightenDir makes dir readable by its owner only (0700) when it is open to
// others, and reports whether it changed anything. A missing dir is left
// alone.
func tightenDir(dir string) (bool, error) {
	fi, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if fi.Mode().Perm()&0o077 == 0 {
		return false, nil
	}
	return true, os.Chmod(dir, 0o700)
}
