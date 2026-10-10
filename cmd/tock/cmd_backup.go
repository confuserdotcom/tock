package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"tock/internal/config"
)

// backupFile is one archive entry and the file it is read from.
type backupFile struct{ name, src string }

// cmdBackup writes one tar.gz with a snapshot of tock.db and the rule,
// config and shell log files. The default path is under the data directory,
// never the current one: the archive holds titles and URLs.
func cmdBackup(env Env, args []string) error {
	fs := newFlags("backup")
	out := fs.String("out", "", "archive path")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("backup takes no arguments")
	}
	st, err := openStore(env)
	if err != nil {
		return err
	}
	defer st.Close()

	path := *out
	if path == "" {
		dir := filepath.Join(env.DataDir, "backups")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
		path = filepath.Join(dir, "tock-"+env.Now().Format("20060102-150405")+".tar.gz")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	n, err := writeBackup(f, env, st.BackupTo, filepath.Dir(path))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "wrote %s (%d files, %s)\n", path, n, size(fi.Size()))
	return nil
}

// writeBackup snapshots the database into a temp directory under tmpParent
// and writes the archive to w. It returns the number of entries.
func writeBackup(w io.Writer, env Env, snapshot func(string) error, tmpParent string) (int, error) {
	tmp, err := os.MkdirTemp(tmpParent, ".tock-backup-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tmp)
	snap := filepath.Join(tmp, "tock.db")
	if err := snapshot(snap); err != nil {
		return 0, err
	}
	return writeArchive(w, []backupFile{
		{"tock.db", snap},
		{"rules.toml", env.rulesPath()},
		{"learned.toml", env.learnedPath()},
		{config.File, filepath.Join(env.ConfigDir, config.File)},
		{"shell.log", env.shellLogPath()},
	}, env.Now())
}

// writeArchive writes the files that exist as flat 0600 tar.gz entries and
// returns how many it wrote.
func writeArchive(w io.Writer, files []backupFile, now time.Time) (int, error) {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	n := 0
	for _, f := range files {
		b, err := os.ReadFile(f.src)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return n, err
		}
		hdr := &tar.Header{Name: f.name, Mode: 0o600, Size: int64(len(b)), ModTime: now, Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			return n, err
		}
		if _, err := tw.Write(b); err != nil {
			return n, err
		}
		n++
	}
	if err := tw.Close(); err != nil {
		return n, err
	}
	return n, gz.Close()
}

// size renders a byte count as "512 B", "12 KB" or "3.4 MB".
func size(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%d KB", b>>10)
	}
	return fmt.Sprintf("%d B", b)
}
