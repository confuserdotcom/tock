package app

import (
	"os"
	"path/filepath"
	"testing"

	"tock/internal/apptest"
)

func TestShellDirsLabelTerminalTime(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	e := apptest.New(t, apptest.Rules+"\n[[rule]]\ndir = '^~/x(/|$)'\nproject = \"ATLAS\"\n")
	e.WriteConfig(t, "[shell]\nterminals = [\"term\"]\n")
	e.Seed(t, apptest.DaySpans()...)
	e.WriteShellLog(t,
		apptest.ShellEvent{At: "08:00", Dir: filepath.Join(home, "y")},
		apptest.ShellEvent{At: "10:40", Dir: filepath.Join(home, "x", "sub")},
	)
	a := open(t, e)
	d, err := a.Day(apptest.Now)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]string{}
	for _, r := range d.Timeline {
		for _, id := range r.IDs {
			got[id] = r.Label.Project
		}
	}
	// span 1 (tock │ …) keeps its title rule; span 2 has no title project
	// and gets ATLAS from its dir.
	if got[1] != "tock" || got[2] != "ATLAS" {
		t.Fatalf("projects = %v", got)
	}
	if a.ShellErr() != nil {
		t.Fatal(a.ShellErr())
	}
}

func TestShellLogUnreadable(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	if err := os.MkdirAll(e.ShellLog, 0o700); err != nil { // a directory cannot be read as a log
		t.Fatal(err)
	}
	a := open(t, e)
	if _, err := a.Day(apptest.Now); err != nil {
		t.Fatalf("a broken shell log must not fail Day: %v", err)
	}
	if a.ShellErr() == nil {
		t.Fatal("ShellErr must report the read error")
	}
}
