//go:build darwin && cgo

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNavPolicy(t *testing.T) {
	for _, c := range []struct {
		name     string
		host     string
		hasHost  bool
		download bool
		want     int
	}{
		{"page link", "127.0.0.1", true, false, navAllow},
		{"localhost", "localhost", true, false, navAllow},
		{"ipv6 loopback", "::1", true, false, navAllow},
		{"export blob", "", false, true, navDownload},
		{"local download", "127.0.0.1", true, true, navDownload},
		{"external link", "example.com", true, false, navBrowser},
		{"external download", "example.com", true, true, navBrowser},
		{"lookalike host", "127.0.0.1.example.com", true, true, navBrowser},
		{"no host, no download", "", false, false, navAllow},
	} {
		if got := navPolicy(c.host, c.hasHost, c.download); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestSaveName(t *testing.T) {
	for in, want := range map[string]string{
		"tock-2026-10-05.csv": "tock-2026-10-05.csv",
		"":                    "tock-export",
		"/":                   "tock-export",
		".":                   "tock-export",
		"..":                  "tock-export",
		"../../etc/passwd":    "passwd",
		"dir/sub/":            "sub",
		"week report.html":    "week report.html",
	} {
		if got := saveName(in); got != want {
			t.Errorf("saveName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFinishSave(t *testing.T) {
	dir := t.TempDir()
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(p string) string {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	dest := filepath.Join(dir, "tock.csv")

	// New file.
	tmp := filepath.Join(dir, "tmp1")
	write(tmp, "first")
	if err := finishSave(tmp, dest); err != nil {
		t.Fatal(err)
	}
	if read(dest) != "first" {
		t.Fatalf("dest %q", read(dest))
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("tmp left behind: %v", err)
	}

	// Replace an existing file.
	tmp = filepath.Join(dir, "tmp2")
	write(tmp, "second")
	if err := finishSave(tmp, dest); err != nil {
		t.Fatal(err)
	}
	if read(dest) != "second" {
		t.Fatalf("replace: dest %q", read(dest))
	}

	// A missing download fails and keeps the old file.
	if err := finishSave(filepath.Join(dir, "gone"), dest); err == nil {
		t.Fatal("missing tmp: want an error")
	}
	if read(dest) != "second" {
		t.Fatalf("failed finish touched dest: %q", read(dest))
	}

	// A folder that isn't there fails too.
	tmp = filepath.Join(dir, "tmp3")
	write(tmp, "third")
	if err := finishSave(tmp, filepath.Join(dir, "no", "such", "x.csv")); err == nil {
		t.Fatal("missing folder: want an error")
	}
}
