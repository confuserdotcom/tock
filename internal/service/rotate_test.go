package service

import (
	"os"
	"path/filepath"
	"testing"
)

// AC4: an 11 MB log becomes log.1 (11 MB) and a fresh, empty log.
func TestRotateLog(t *testing.T) {
	dir := t.TempDir()
	big, small := filepath.Join(dir, "trackd.log"), filepath.Join(dir, "small.log")
	if err := os.WriteFile(big, make([]byte, 11<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(big+".1", []byte("older"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(small, make([]byte, 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	// An append-mode writer opened before rotation, like launchd's.
	w, err := os.OpenFile(big, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, p := range []string{big, small, filepath.Join(dir, "missing.log")} {
		if err := RotateLog(p, LogMaxBytes); err != nil {
			t.Fatalf("RotateLog(%s): %v", p, err)
		}
	}
	if fi, _ := os.Stat(big + ".1"); fi == nil || fi.Size() != 11<<20 || fi.Mode().Perm() != 0o600 {
		t.Fatalf("log.1 = %v", fi)
	}
	if fi, _ := os.Stat(big); fi.Size() != 0 {
		t.Fatalf("log size = %d; want 0", fi.Size())
	}
	if fi, _ := os.Stat(small); fi.Size() != 1024 {
		t.Fatalf("small log rotated")
	}
	if _, err := os.Stat(filepath.Join(dir, "small.log.1")); !os.IsNotExist(err) {
		t.Fatal("small log got a .1")
	}
	// The old descriptor now writes at the start of the fresh log.
	if _, err := w.WriteString("next line\n"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(big); string(b) != "next line\n" {
		t.Fatalf("log after write = %q", b)
	}
}
