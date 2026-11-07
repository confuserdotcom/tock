package snapshot

import (
	"compress/gzip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// ErrLowDisk stops a snapshot when free space is under twice the live DB.
var ErrLowDisk = errors.New("snapshot: not enough free disk space")

// DamagedError says the live database (or its copy) failed to read or
// check.
type DamagedError struct{ Detail string }

func (e *DamagedError) Error() string { return "snapshot: database damaged: " + e.Detail }

// Opts is one snapshot to write. Vacuum writes a consistent copy of the
// live database to a path that does not exist (VACUUM INTO).
type Opts struct {
	Dir, Name string
	LiveSize  int64
	Free      func(dir string) (uint64, error)
	Vacuum    func(path string) error
}

// damaged reports whether err is SQLite saying the file is broken.
func damaged(err error) bool {
	s := err.Error()
	return strings.Contains(s, "malformed") || strings.Contains(s, "not a database") || strings.Contains(s, "corrupt")
}

// Write copies the live database into o.Dir/o.Name: VACUUM INTO a temp
// file, quick_check it, gzip it to a temp name, fsync, rename. Nothing but
// the finished snapshot is left behind.
func Write(o Opts) (Info, error) {
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return Info{}, fmt.Errorf("snapshot: mkdir: %w", err)
	}
	if err := os.Chmod(o.Dir, 0o700); err != nil {
		return Info{}, fmt.Errorf("snapshot: chmod dir: %w", err)
	}
	if o.LiveSize > 0 && o.Free != nil {
		free, err := o.Free(o.Dir)
		if err != nil {
			return Info{}, fmt.Errorf("snapshot: free space: %w", err)
		}
		if free < uint64(2*o.LiveSize) {
			return Info{}, ErrLowDisk
		}
	}
	raw := filepath.Join(o.Dir, "."+o.Name+".db.tmp")
	gz := filepath.Join(o.Dir, "."+o.Name+".tmp")
	_ = os.Remove(raw)
	_ = os.Remove(gz)
	defer os.Remove(raw)
	if err := o.Vacuum(raw); err != nil {
		if damaged(err) {
			return Info{}, &DamagedError{Detail: err.Error()}
		}
		return Info{}, fmt.Errorf("snapshot: copy: %w", err)
	}
	res, err := Check(raw)
	if err != nil {
		return Info{}, err
	}
	if res != "ok" {
		return Info{}, &DamagedError{Detail: res}
	}
	if err := compress(raw, gz); err != nil {
		os.Remove(gz)
		return Info{}, err
	}
	final := filepath.Join(o.Dir, o.Name)
	if err := os.Rename(gz, final); err != nil {
		os.Remove(gz)
		return Info{}, fmt.Errorf("snapshot: rename: %w", err)
	}
	fi, err := os.Stat(final)
	if err != nil {
		return Info{}, fmt.Errorf("snapshot: %w", err)
	}
	info, _ := Parse(o.Name)
	info.Path, info.Size = final, fi.Size()
	if info.Kind == Pre {
		info.AtMS = fi.ModTime().UnixMilli()
	}
	return info, nil
}

// compress gzips src into dst (0600, fsynced).
func compress(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	zw := gzip.NewWriter(out)
	_, err = io.Copy(zw, in)
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if serr := out.Sync(); err == nil {
		err = serr
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("snapshot: compress: %w", err)
	}
	return nil
}

// Check runs PRAGMA quick_check on the database at path, read-only. It
// returns "ok" or the first problem line.
func Check(path string) (string, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return "", fmt.Errorf("snapshot: check: %w", err)
	}
	defer db.Close()
	var res string
	if err := db.QueryRow(`PRAGMA quick_check(1)`).Scan(&res); err != nil {
		if damaged(err) {
			return err.Error(), nil
		}
		return "", fmt.Errorf("snapshot: check: %w", err)
	}
	return res, nil
}

// List returns the snapshots in dir, unsorted. A missing dir has none.
func List(dir string) ([]Info, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("snapshot: list: %w", err)
	}
	var out []Info
	for _, e := range entries {
		info, ok := Parse(e.Name())
		if !ok || e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		info.Path, info.Size = filepath.Join(dir, e.Name()), fi.Size()
		if info.Kind == Pre {
			info.AtMS = fi.ModTime().UnixMilli()
		}
		out = append(out, info)
	}
	return out, nil
}

// Extract gunzips src to dst (which must not exist) and checks it. On any
// failure dst is removed.
func Extract(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("snapshot: extract: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("snapshot: extract: %w", err)
	}
	defer func() {
		if err != nil {
			os.Remove(dst)
		}
	}()
	zr, err := gzip.NewReader(in)
	if err != nil {
		out.Close()
		return fmt.Errorf("snapshot: extract: %w", err)
	}
	_, err = io.Copy(out, zr)
	if serr := out.Sync(); err == nil {
		err = serr
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("snapshot: extract: %w", err)
	}
	res, err := Check(dst)
	if err != nil {
		return err
	}
	if res != "ok" {
		return &DamagedError{Detail: res}
	}
	return nil
}
