package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// SaveFeatures writes changes (feature id to value) into the config.toml at
// path, keeping everything else in it. The result must load, or nothing is
// written. The old file is kept as config.toml.bak. Ids not in Features are
// ignored. A group turned on loses its key instead, as on is its default.
func SaveFeatures(path string, changes map[string]bool) error {
	return saveEdit(path, "save features", func(b []byte, _ Config) ([]byte, error) {
		var err error
		for _, f := range Features {
			v, ok := changes[f.ID]
			if !ok {
				continue
			}
			if f.IsGroup && v {
				// Groups are on by default: on drops the key, so off then
				// on gives back the file as it was.
				b, err = UnsetKey(b, f.Table, f.Key)
			} else {
				b, err = SetBool(b, f.Table, f.Key, v)
			}
			if err != nil {
				return nil, err
			}
		}
		return b, nil
	})
}

// Hidden kinds for SetHidden.
const (
	HideApp  = "app"
	HideSite = "site"
)

// SetHidden adds value to (hide) or removes it from (!hide) the [privacy]
// hide_apps or hide_sites list of the config.toml at path, the same way
// SaveFeatures writes.
func SetHidden(path, kind, value string, hide bool) error {
	key := map[string]string{HideApp: "hide_apps", HideSite: "hide_sites"}[kind]
	if key == "" {
		return fmt.Errorf("config: hide: kind %q, want %s or %s", kind, HideApp, HideSite)
	}
	return saveEdit(path, "hide", func(b []byte, c Config) ([]byte, error) {
		l := c.Privacy.HideApps
		if kind == HideSite {
			l = c.Privacy.HideSites
		}
		has := slices.Contains(l, value)
		switch {
		case hide && has, !hide && !has:
			return b, nil
		case hide:
			l = append(slices.Clone(l), value)
		default:
			l = slices.DeleteFunc(slices.Clone(l), func(s string) bool { return s == value })
		}
		return SetList(b, "privacy", key, l)
	})
}

// saveEdit runs edit on the config.toml at path (its bytes and settings)
// and writes the result: the file must load before and after, the old one
// is kept as config.toml.bak, and the write is a rename.
func saveEdit(path, what string, edit func([]byte, Config) ([]byte, error)) error {
	src, err := os.ReadFile(path)
	existed := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("config: %s: %w", what, err)
	}
	c := Default()
	if existed {
		if c, err = Parse(File, src); err != nil {
			return fmt.Errorf("config: fix config.toml first: %w", err)
		}
	}
	b, err := edit(src, c)
	if err != nil {
		return err
	}
	if existed && string(b) == string(src) {
		return nil
	}
	if _, err := Parse(File, b); err != nil {
		return fmt.Errorf("config: %s: %w", what, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: %s: %w", what, err)
	}
	if existed {
		if err := os.WriteFile(path+".bak", src, 0o600); err != nil {
			return fmt.Errorf("config: %s: backup: %w", what, err)
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("config: %s: %w", what, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("config: %s: %w", what, err)
	}
	return nil
}
