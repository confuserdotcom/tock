package main

import (
	"os"
	"path/filepath"
	"testing"

	"tock/internal/testfs"
)

func TestTockBinFindsBrewsTock(t *testing.T) {
	h := t.TempDir()
	testfs.SetHome(t, h)
	t.Setenv("TOCK_BIN", "")
	brew := filepath.Join(t.TempDir(), "tock")
	if err := os.WriteFile(brew, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(h, ".local", "bin", "tock")
	if got := findTock([]string{local, brew}); got != brew {
		t.Fatalf("findTock = %q, want the one that exists, %q", got, brew)
	}
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findTock([]string{local, brew}); got != local {
		t.Fatalf("findTock = %q, want the first, %q", got, local)
	}
	if got := findTock([]string{filepath.Join(h, "nope")}); got != filepath.Join(h, "nope") {
		t.Fatalf("with none there findTock = %q, want the first, for the error to name", got)
	}
	t.Setenv("TOCK_BIN", "/x/tock")
	if got := tockBin(); got != "/x/tock" {
		t.Fatalf("tockBin with TOCK_BIN = %q", got)
	}
}
