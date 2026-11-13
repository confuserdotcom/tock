package main

import "os"

// detach starts name on its own (see startDetached) and lets it run on.
func detach(name string, args []string, out *os.File) error {
	c, err := startDetached(name, args, out)
	if err != nil {
		return err
	}
	return c.Process.Release()
}
