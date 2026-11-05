package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SaveFeatures writes changes (feature id to value) into the config.toml at
// path, keeping everything else in it. The result must load, or nothing is
// written. The old file is kept as config.toml.bak. Ids not in Features are
// ignored.
func SaveFeatures(path string, changes map[string]bool) error {
	src, err := os.ReadFile(path)
	existed := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("config: save features: %w", err)
	}
	if existed {
		if _, err := Parse(File, src); err != nil {
			return fmt.Errorf("config: fix config.toml first: %w", err)
		}
	}
	b := src
	for _, f := range Features {
		v, ok := changes[f.ID]
		if !ok {
			continue
		}
		if b, err = SetBool(b, f.Table, f.Key, v); err != nil {
			return err
		}
	}
	if _, err := Parse(File, b); err != nil {
		return fmt.Errorf("config: save features: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: save features: %w", err)
	}
	if existed {
		if err := os.WriteFile(path+".bak", src, 0o600); err != nil {
			return fmt.Errorf("config: save features: backup: %w", err)
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("config: save features: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("config: save features: %w", err)
	}
	return nil
}
