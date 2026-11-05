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

func TestSetHidden(t *testing.T) {
	path := filepath.Join(t.TempDir(), File)
	src := "# mine\n[privacy]\nkeep_titles = 30\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		kind, v string
		hide    bool
	}{
		{HideApp, "com.example.diary", true},
		{HideSite, "bank.com", true},
		{HideSite, "bank.com", true}, // already there
		{HideSite, "mail.com", true},
		{HideSite, "bank.com", false},
		{HideApp, "com.other", false}, // not there
	} {
		if err := SetHidden(path, step.kind, step.v, step.hide); err != nil {
			t.Fatalf("%+v: %v", step, err)
		}
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(c.Privacy.HideApps, ",") != "com.example.diary" || strings.Join(c.Privacy.HideSites, ",") != "mail.com" {
		t.Fatalf("privacy = %+v", c.Privacy)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "# mine") || c.Privacy.KeepTitles != 30 {
		t.Fatalf("file:\n%s", b)
	}
	if err := SetHidden(path, HideSite, "Bad Host", true); err == nil {
		t.Error("an invalid site was saved")
	}
	if err := SetHidden(path, "window", "x", true); err == nil {
		t.Error("an unknown kind was accepted")
	}
}
