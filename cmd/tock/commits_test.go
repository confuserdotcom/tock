package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

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
		for _, s := range secs {
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
	te, _ := doctorEnv(t, strings.Replace(repoRules, "name = \"ATLAS\"\n", "name = \"ATLAS\"\nrepos = [\"/r/gone\"]\n", 1))
	te.seed(t, daySpans()...)
	setStates(t, te, map[string]string{store.KeyHeartbeat: nowMS(), store.KeyAXTrusted: "1"})
	te.Git = fakeGit()
	code, out := te.run(t, "", "doctor")
	if code != 1 || !strings.Contains(out, "!!  git: project ATLAS: /r/gone is not a readable git repo") ||
		strings.Contains(out, "/r/tock") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	te2, _ := doctorEnv(t, repoRules)
	te2.seed(t, daySpans()...)
	setStates(t, te2, map[string]string{store.KeyHeartbeat: nowMS(), store.KeyAXTrusted: "1"})
	te2.Git = fakeGit()
	if code, out := te2.run(t, "", "doctor"); code != 0 || !strings.Contains(out, "ok  git: 1 repo\n") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}
