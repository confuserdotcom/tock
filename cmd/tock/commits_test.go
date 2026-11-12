package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tock/internal/store"
)

// fakeGit serves git for repo "/r/tock" only: log answers with the author
// times in secs.
func fakeGit(secs ...int64) func(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if dir != "/r/tock" {
			return nil, errors.New("exit status 128")
		}
		switch args[0] {
		case "config":
			return nil, errors.New("exit status 1")
		case "rev-parse":
			return []byte(".git\n"), nil
		}
		var b strings.Builder
		subj := strings.Contains(strings.Join(args, " "), "%s")
		for i, s := range secs {
			if subj {
				fmt.Fprintf(&b, "%d\x1fc%d\x1fChange %d\n", s, i, i)
				continue
			}
			fmt.Fprintf(&b, "%d\n", s)
		}
		return []byte(b.String()), nil
	}
}

var repoRules = strings.Replace(testRules, "name = \"tock\"\n", "name = \"tock\"\nrepos = [\"/r/tock\"]\n", 1)

func TestTodayCommits(t *testing.T) {
	te := newTestEnv(t, repoRules)
	te.seed(t, daySpans()...)
	te.Git = fakeGit(at(t, "09:30")/1000, at(t, "10:05")/1000)
	code, out := te.run(t, "", "today")
	if code != 0 || !strings.Contains(out, "commits  tock 2\n") {
		t.Fatalf("exit %d:\n%s%s", code, out, te.errb)
	}
	code, out = te.run(t, "", "review", "--week", "2026-10-05")
	if code != 0 || !strings.Contains(out, "commits  tock 2\n") {
		t.Fatalf("review exit %d:\n%s%s", code, out, te.errb)
	}
}

func TestDoctorRepos(t *testing.T) {
	te, _ := doctorEnv(t, strings.Replace(repoRules, "name = \"ATLAS\"\n", "name = \"ATLAS\"\nrepos = [\"/r/broken\", \"/r/gone\"]\n", 1))
	te.seed(t, daySpans()...)
	setStates(t, te, map[string]string{store.KeyHeartbeat: nowMS(), store.KeyAXTrusted: "1"})
	te.Git = fakeGit()
	te.DirExists = func(dir string) bool { return dir != "/r/gone" }
	code, out := te.run(t, "", "doctor")
	if code != 1 || !strings.Contains(out, "!!  git: project ATLAS: /r/broken is not a readable git repo") ||
		strings.Contains(out, "/r/tock") || strings.Contains(out, "/r/gone") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	// A repo that is not on this machine (rules.toml synced from another)
	// is no problem.
	te3, _ := doctorEnv(t, strings.Replace(repoRules, "name = \"ATLAS\"\n", "name = \"ATLAS\"\nrepos = [\"/r/gone\"]\n", 1))
	te3.seed(t, daySpans()...)
	setStates(t, te3, map[string]string{store.KeyHeartbeat: nowMS(), store.KeyAXTrusted: "1"})
	te3.Git = fakeGit()
	te3.DirExists = func(dir string) bool { return dir != "/r/gone" }
	if code, out := te3.run(t, "", "doctor"); code != 0 || !strings.Contains(out, "ok  git: 1 repo, 1 not on this machine\n") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	te2, _ := doctorEnv(t, repoRules)
	te2.seed(t, daySpans()...)
	setStates(t, te2, map[string]string{store.KeyHeartbeat: nowMS(), store.KeyAXTrusted: "1"})
	te2.Git = fakeGit()
	te2.DirExists = func(string) bool { return true }
	if code, out := te2.run(t, "", "doctor"); code != 0 || !strings.Contains(out, "ok  git: 1 repo\n") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}

func TestCommitsCmd(t *testing.T) {
	te := newTestEnv(t, repoRules)
	te.seed(t, daySpans()...)
	te.Git = fakeGit(at(t, "09:30")/1000, at(t, "10:05")/1000)
	code, out := te.run(t, "", "commits")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, te.errb)
	}
	for _, want := range []string{
		"09:30  tock  c0    30m  Change 0\n",
		"10:05  tock  c1    35m  Change 1\n",
		"since last commit  tock 35m\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "2 commits · 1h05 of your time behind them\n") || strings.Contains(out, "agent") {
		t.Errorf("summary or an agent column without agents:\n%s", out)
	}
	code, out = te.run(t, "", "commits", "--days", "2", "--project", "ATLAS")
	if code != 0 || !strings.Contains(out, "no commits") {
		t.Fatalf("filtered: exit %d:\n%s", code, out)
	}
	if code, _ := te.run(t, "", "commits", "--days", "0"); code != 2 {
		t.Fatalf("bad days: %d", code)
	}
}

func TestCommitsWithAgents(t *testing.T) {
	te := newTestEnv(t, repoRules)
	te.seed(t, daySpans()...)
	te.Git = fakeGit(at(t, "09:30")/1000, at(t, "10:05")/1000)
	dir := t.TempDir()
	te.AgentDir = dir
	var b strings.Builder
	for ms := at(t, "09:40"); ms <= at(t, "10:00"); ms += 2 * 60_000 { // an agent in /r/tock
		ts := time.UnixMilli(ms).UTC()
		fmt.Fprintf(&b, `{"type":"assistant","timestamp":%q,"cwd":"/r/tock","sessionId":"s"}`+"\n", ts.Format(time.RFC3339Nano))
	}
	os.MkdirAll(filepath.Join(dir, "p"), 0o700)
	os.WriteFile(filepath.Join(dir, "p", "s.jsonl"), []byte(b.String()), 0o600)
	code, out := te.run(t, "", "commits")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, te.errb)
	}
	for _, want := range []string{"  you  agent\n", "10:05  tock  c1    35m    20m  Change 1\n", "of your time behind them, 20m of agents'"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
}
