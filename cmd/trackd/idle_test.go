package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tock/internal/collect"
)

func TestIdleHooks(t *testing.T) {
	dir := t.TempDir()
	now := time.UnixMilli(10_000_000)
	path := filepath.Join(dir, collect.IdleFile)
	if err := runIdleHook("idle", []string{"--after", "300"}, dir, now); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); strings.TrimSpace(string(b)) != "9700000" {
		t.Fatalf("idle file = %q", b)
	}
	if err := runIdleHook("active", nil, dir, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("active must clear the file")
	}
	if err := runIdleHook("idle", nil, dir, now); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{{"--after", "-1"}, {"x"}} {
		if err := runIdleHook("idle", bad, dir, now); err == nil {
			t.Errorf("idle %v accepted", bad)
		}
	}
	if err := runIdleHook("active", []string{"x"}, dir, now); err == nil {
		t.Error("active x accepted")
	}
}
