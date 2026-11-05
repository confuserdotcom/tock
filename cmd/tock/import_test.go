package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// awExport is an ActivityWatch export of 2026-10-10 in UTC: a terminal for
// an hour, then YouTube in a browser for 20 minutes.
const awExport = `{"buckets": {
 "aw-watcher-window_mac": {"type": "currentwindow", "events": [
  {"timestamp": "2026-10-10T07:00:00Z", "duration": 3600, "data": {"app": "term", "title": "make test"}},
  {"timestamp": "2026-10-10T08:00:00Z", "duration": 1200, "data": {"app": "Firefox", "title": "x"}}]},
 "aw-watcher-web-firefox": {"type": "web.tab.current", "events": [
  {"timestamp": "2026-10-10T08:00:00Z", "duration": 1200, "data": {"url": "https://www.youtube.com/watch", "title": "cats"}}]}
}}`

func TestImportActivityWatch(t *testing.T) {
	te := newTestEnv(t, testRules)
	file := filepath.Join(t.TempDir(), "aw.json")
	if err := os.WriteFile(file, []byte(awExport), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out := te.run(t, "", "import", "activitywatch", file)
	if code != 0 || out != "imported 2 views over 1 day from ActivityWatch\n" {
		t.Fatalf("import: code %d out %q err %q", code, out, te.errb)
	}
	code, out = te.run(t, "", "today", "--date", "2026-10-10")
	if code != 0 || !strings.Contains(out, "Code") || !strings.Contains(out, "Entertainment") {
		t.Fatalf("today after import: code %d\n%s", code, out)
	}
	// Importing again replaces the days, never doubles them.
	if code, out := te.run(t, "", "import", "activitywatch", file); code != 0 || !strings.Contains(out, "2 views") {
		t.Fatalf("again: code %d out %q", code, out)
	}
	if _, out := te.run(t, "", "doctor"); strings.Contains(out, "activitywatch") {
		t.Errorf("doctor names the import:\n%s", out)
	}
	if code, _ := te.run(t, "", "import", "toggl", file); code != 2 {
		t.Errorf("unknown source: code %d", code)
	}
	if code, _ := te.run(t, "", "import", "activitywatch"); code != 2 {
		t.Errorf("no file: code %d", code)
	}
}

func TestImportActivityWatchHidden(t *testing.T) {
	te := newTestEnv(t, testRules)
	if err := os.WriteFile(filepath.Join(te.ConfigDir, "config.toml"), []byte("[privacy]\nhide_sites = [\"youtube.com\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "aw.json")
	if err := os.WriteFile(file, []byte(awExport), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := te.run(t, "", "import", "activitywatch", file); code != 0 {
		t.Fatalf("import: code %d err %q", code, te.errb)
	}
	// The hidden site keeps neither its URL nor its title, so no rule sees it.
	if code, out := te.run(t, "", "today", "--date", "2026-10-10"); code != 0 || strings.Contains(out, "Entertainment") {
		t.Fatalf("hidden site labelled: code %d\n%s", code, out)
	}
}
