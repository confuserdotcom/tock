package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveFeatures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tock", File)
	// no file yet: created, with its directory
	if err := SaveFeatures(path, map[string]bool{"break_screen": true}); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil || !c.Focus.BreakScreen {
		t.Fatalf("after first save: break_screen %v, %v", c.Focus.BreakScreen, err)
	}
	// existing file: comments kept, backup written, unknown id ignored
	src := "# mine\n[focus]\nbreak_screen = true # keep\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveFeatures(path, map[string]bool{"break_screen": false, "nope": true}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "# mine") || !strings.Contains(string(b), "break_screen = false") {
		t.Fatalf("file:\n%s", b)
	}
	if bak, _ := os.ReadFile(path + ".bak"); string(bak) != src {
		t.Fatalf("backup = %q", bak)
	}
	// a broken file is never touched
	if err := os.WriteFile(path, []byte("[focus\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveFeatures(path, map[string]bool{"break_screen": true}); err == nil {
		t.Fatal("broken file saved")
	}
	if b, _ := os.ReadFile(path); string(b) != "[focus\n" {
		t.Fatalf("broken file changed: %q", b)
	}
}
