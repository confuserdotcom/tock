package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"tock/internal/store"
)

// untar extracts path into a temp dir and returns the entry names, sorted.
func untar(t *testing.T, path string) (string, []string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Mode != 0o600 || strings.Contains(h.Name, "/") {
			t.Fatalf("entry %q mode %o", h.Name, h.Mode)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, h.Name), b, 0o600); err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
	sort.Strings(names)
	return dir, names
}

func TestBackupDefaultPath(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, seedSpan{id: 1, start: "09:00", end: "10:00", bundle: "term", app: "WezTerm", title: "tock │ x"})
	if err := os.WriteFile(filepath.Join(te.ConfigDir, "config.toml"), []byte("[focus]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out := te.run(t, "", "backup")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, te.errb)
	}
	want := filepath.Join(te.DataDir, "backups", "tock-20261010-150000.tar.gz")
	if !strings.HasPrefix(out, "wrote "+want+" (3 files, ") {
		t.Fatalf("out = %q", out)
	}
	fi, err := os.Stat(want)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("archive mode: %v %v", fi, err)
	}
	if di, err := os.Stat(filepath.Dir(want)); err != nil || di.Mode().Perm() != 0o700 {
		t.Fatalf("backups dir mode: %v %v", di, err)
	}
	dir, names := untar(t, want)
	if got := strings.Join(names, " "); got != "config.toml rules.toml tock.db" {
		t.Fatalf("entries = %s", got)
	}
	st, err := store.Open(filepath.Join(dir, "tock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if n, err := st.NextID(); err != nil || n != 2 {
		t.Fatalf("snapshot NextID = %d, %v", n, err)
	}
	if ents, _ := os.ReadDir(filepath.Dir(want)); len(ents) != 1 {
		t.Fatalf("temp files left behind: %v", ents)
	}
}

func TestBackupOutAndNoOverwrite(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t)
	if err := os.MkdirAll(te.StateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(te.shellLogPath(), []byte("1\t/x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "b.tar.gz")
	if code, _ := te.run(t, "", "backup", "--out", out); code != 0 {
		t.Fatalf("exit %d: %s", code, te.errb)
	}
	_, names := untar(t, out)
	if got := strings.Join(names, " "); got != "rules.toml shell.log tock.db" {
		t.Fatalf("entries = %s", got)
	}
	before, _ := os.ReadFile(out)
	if code, _ := te.run(t, "", "backup", "--out", out); code != 1 {
		t.Fatalf("second backup exit %d, want 1", code)
	}
	if after, _ := os.ReadFile(out); string(after) != string(before) {
		t.Fatal("existing archive was changed")
	}
}

func TestBackupNoDB(t *testing.T) {
	te := newTestEnv(t, testRules)
	code, _ := te.run(t, "", "backup")
	if code != 1 || !strings.Contains(te.errb.String(), "no data yet") {
		t.Fatalf("exit %d: %s", code, te.errb)
	}
	if code, _ := te.run(t, "", "backup", "extra"); code != 2 {
		t.Fatalf("extra arg exit %d, want 2", code)
	}
}
