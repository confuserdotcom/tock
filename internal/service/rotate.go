package service

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// LogMaxBytes is the size above which a service log is rotated at start.
const LogMaxBytes = 10 << 20

// RotateLog keeps one old log: when path is larger than max it copies it
// to path.1 (replacing the older one) and empties path. It copies rather
// than renames because launchd and systemd hold their own append-mode
// descriptor to path: after a rename they would go on writing to path.1.
// Run it at start, before the program logs. A missing file is fine.
func RotateLog(path string, max int64) error {
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Size() <= max {
		return nil
	}
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := path + ".1.tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path+".1")
	}
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rotate %s: %w", path, err)
	}
	return os.Truncate(path, 0)
}
