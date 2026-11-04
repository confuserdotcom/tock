package classify

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// Starter is the rules.toml that `tock init` writes.
//
//go:embed starter.toml
var Starter []byte

// WriteStarter writes the starter rules to path, creating its directory. It
// never overwrites: an existing file is an error wrapping os.ErrExist.
func WriteStarter(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("classify: starter dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("classify: starter: %w", err)
	}
	if _, err := f.Write(Starter); err != nil {
		f.Close()
		os.Remove(path)
		return fmt.Errorf("classify: starter: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("classify: starter: %w", err)
	}
	return nil
}
