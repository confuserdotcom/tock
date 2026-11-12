package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
)

func TestAgents(t *testing.T) {
	e := apptest.New(t, strings.Replace(repoRules, "name = \"tock\"\n", "name = \"tock\"\nrepos = [\"/r/tock\"]\n", 1))
	e.Seed(t, apptest.DaySpans()...)
	dir := t.TempDir()
	line := func(typ, at, cwd, extra string) string {
		ts := time.UnixMilli(apptest.At(t, at, 0)).UTC().Format(time.RFC3339Nano)
		return fmt.Sprintf(`{"type":%q,"timestamp":%q,"cwd":%q,"sessionId":"s1"%s}`+"\n", typ, ts, cwd, extra)
	}
	var b strings.Builder
	b.WriteString(line("user", "09:10", "/r/tock/web", `,"origin":{"kind":"human"}`))
	for m := 11; m <= 30; m++ {
		b.WriteString(line("assistant", fmt.Sprintf("09:%02d", m), "/r/tock/web", ""))
	}
	b.WriteString(`{"type":"ai-title","aiTitle":"tock │","sessionId":"s1"}` + "\n")
	if err := os.MkdirAll(filepath.Join(dir, "-r-tock"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "-r-tock", "s1.jsonl"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := Open(Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config, ShellLog: e.ShellLog, Agents: dir}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	from := a.midnight(e.Clock)
	got, err := a.Agents(from, from.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("rows = %+v", got.Rows)
	}
	r := got.Rows[0]
	// 09:10 then 09:11..09:30: one busy stretch of 20m. The 09:00-10:40
	// terminal view is titled "tock │ ✳ x", which holds the session title: 1h40 of yours.
	if r.Project != "tock" || r.AgentMS != 20*60_000 || r.Prompts != 1 || r.Sessions != 1 || r.YouMS != 100*60_000 {
		t.Fatalf("row = %+v", r)
	}
	for dir, want := range map[string]string{
		"/elsewhere/x": "/elsewhere/x", "": "(unknown)",
		"/private/tmp/claude-501/x/scratchpad": TempDirs, "/tmp/a": TempDirs,
		"/private/var/folders/xf/zp7/T/essai/Licence": TempDirs, "/var/folders/ab/T/x": TempDirs,
	} {
		if got := a.projectOfDir(dir); got != want {
			t.Errorf("projectOfDir(%q) = %q, want %q", dir, got, want)
		}
	}
}
