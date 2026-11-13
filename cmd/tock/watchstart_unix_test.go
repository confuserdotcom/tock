//go:build unix

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWatchStart(t *testing.T) {
	out := filepath.Join(t.TempDir(), "trackd.out")
	run := func(script string) error {
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		c, err := startDetached("/bin/sh", []string{"-c", script}, f)
		if err != nil {
			t.Fatal(err)
		}
		return watchStart(c, out, 500*time.Millisecond)
	}
	// One that dies at once: tock start says so, with what it wrote.
	if err := run("echo 'config.toml: unknown key'; exit 3"); err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("dead trackd: %v", err)
	}
	// One that keeps running is let go.
	if err := run("sleep 2"); err != nil {
		t.Fatalf("running trackd: %v", err)
	}
}
