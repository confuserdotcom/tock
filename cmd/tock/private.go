package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeOwn writes data to name in dir under tock's state dir, a folder
// only you can read, and returns the path. It writes a temporary file next
// to it, syncs it and renames it over, so a symlink at the path is
// replaced, never written through, and a crash leaves the old file or the
// new one. Generated files used to go to the shared temp dir under a
// guessable name, where another user's symlink could redirect them.
func writeOwn(env Env, dir, name string, data []byte) (string, error) {
	d := filepath.Join(env.StateDir, dir)
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", fmt.Errorf("%s: %w", d, err)
	}
	// Made earlier with a looser mode: close it. A symlink you put there is
	// yours to manage, and is left alone.
	if fi, err := os.Lstat(d); err == nil && fi.IsDir() && fi.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(d, 0o700); err != nil {
			return "", fmt.Errorf("%s: %w", d, err)
		}
	}
	f, err := os.CreateTemp(d, ".tmp-*") // 0600
	if err != nil {
		return "", fmt.Errorf("write %s: %w", filepath.Join(d, name), err)
	}
	tmp := f.Name()
	defer os.Remove(tmp) // gone already after a successful rename
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("write %s: %w", filepath.Join(d, name), err)
	}
	path := filepath.Join(d, name)
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("write %s: %w", filepath.Join(d, name), err)
	}
	return path, nil
}
