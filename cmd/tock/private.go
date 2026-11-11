package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeOwn writes data to name in dir under tock's state dir, a folder
// only you can read, and returns the path. It writes a temporary file next
// to it and renames it over, so a symlink at the path is replaced, never
// written through. Generated files used to go to the shared temp dir under
// a guessable name, where another user's symlink could redirect them.
func writeOwn(env Env, dir, name string, data []byte) (string, error) {
	d := filepath.Join(env.StateDir, dir)
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", fmt.Errorf("%s: %w", dir, err)
	}
	f, err := os.CreateTemp(d, ".tmp-*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // after a successful rename it is gone already
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	path := filepath.Join(d, name)
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}
