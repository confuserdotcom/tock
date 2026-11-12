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

func TestAgentDirsLabelTerminalTime(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	e := apptest.New(t, apptest.Rules+"\n[[project]]\nname = \"Proj\"\nrepos = [\"~/proj\"]\n")
	e.WriteConfig(t, "[shell]\nterminals = [\"term\"]\n")
	e.Seed(t, apptest.DaySpans()...)
	logs := t.TempDir()
	var b strings.Builder
	for _, hhmm := range []string{"10:45", "10:50", "10:55"} {
		ts := time.UnixMilli(apptest.At(t, hhmm, 0)).UTC().Format(time.RFC3339Nano)
		fmt.Fprintf(&b, `{"type":"assistant","timestamp":%q,"cwd":%q,"sessionId":"s"}`+"\n", ts, filepath.Join(home, "proj", "web"))
	}
	b.WriteString(`{"type":"ai-title","aiTitle":"ATLAS │ y","sessionId":"s"}` + "\n")
	os.MkdirAll(filepath.Join(logs, "p"), 0o700)
	if err := os.WriteFile(filepath.Join(logs, "p", "s.jsonl"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	projects := func(index string) map[int64]string {
		a, err := Open(Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config, ShellLog: e.ShellLog,
			Agents: logs, AgentIndex: index}, e.NowFunc())
		if err != nil {
			t.Fatal(err)
		}
		defer a.Close()
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
		return got
	}
	index := filepath.Join(t.TempDir(), "agents.index")
	// Span 2 (10:40-11:14, "ATLAS │ y") shows the session, which ran in
	// ~/proj/web: Proj. Span 1 keeps its title rule.
	if got := projects(index); got[2] != "Proj" || got[1] != "tock" {
		t.Fatalf("projects = %v", got)
	}
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("index not kept: %v", err)
	}
	// Without an index path (trackd, tockbar) the logs are not read.
	if got := projects(""); got[2] != "" {
		t.Fatalf("without an index: %v", got)
	}
	// agents = false turns it off.
	e.WriteConfig(t, "[shell]\nterminals = [\"term\"]\nagents = false\n")
	if got := projects(index); got[2] != "" {
		t.Fatalf("agents off: %v", got)
	}
}
