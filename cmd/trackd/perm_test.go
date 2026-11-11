package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A data or state dir someone made with a loose umask (make update did) is
// closed to other users at start: the privacy page promises 0700.
func TestTightenDir(t *testing.T) {
	d := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(d, 0o755); err != nil { // the umask may have masked it
		t.Fatal(err)
	}
	changed, err := tightenDir(d)
	if err != nil || !changed {
		t.Fatalf("tighten: %v %v", changed, err)
	}
	if fi, _ := os.Stat(d); fi.Mode().Perm() != 0o700 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	if changed, err := tightenDir(d); err != nil || changed {
		t.Fatalf("second call: %v %v", changed, err)
	}
	if changed, err := tightenDir(filepath.Join(d, "missing")); err != nil || changed {
		t.Fatalf("missing dir: %v %v", changed, err)
	}
}
