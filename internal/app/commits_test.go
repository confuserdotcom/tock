package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/report"
)

// gitFake answers git log with fixed author times per repo and counts the
// log runs.
type gitFake struct {
	times map[string][]int64 // repo -> unix seconds
	runs  int
}

func (g *gitFake) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	switch args[0] {
	case "config":
		return nil, errors.New("exit status 1")
	case "rev-parse":
		if _, ok := g.times[dir]; !ok {
			return nil, errors.New("not a git repository")
		}
		return []byte(".git\n"), nil
	}
	g.runs++
	ts, ok := g.times[dir]
	if !ok {
		return nil, errors.New("not a git repository")
	}
	var b strings.Builder
	for _, s := range ts {
		fmt.Fprintf(&b, "%d\n", s)
	}
	return []byte(b.String()), nil
}

const repoRules = apptest.Rules + `
[[project]]
name = "Repo"
repos = ["/r/one", "/r/two"]
`

func TestCommitsTodayAndWeek(t *testing.T) {
	e := apptest.New(t, strings.Replace(repoRules, "name = \"tock\"\n", "name = \"tock\"\nrepos = [\"/r/tock\"]\n", 1))
	e.Seed(t, apptest.DaySpans()...)
	sec := func(hhmm string, day int) int64 { return apptest.At(t, hhmm, day) / 1000 }
	g := &gitFake{times: map[string][]int64{
		"/r/tock": {sec("09:30", 0), sec("10:00", 0), sec("10:30", -1)},
		"/r/one":  {sec("11:00", 0)},
		// /r/two is missing: it counts nothing.
	}}
	a := open(t, e)
	a.SetGitRunner(g.run)

	d, err := a.Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if want := []report.NameN{{Name: "tock", N: 2}, {Name: "Repo", N: 1}}; !reflect.DeepEqual(d.Commits, want) {
		t.Fatalf("day commits = %v", d.Commits)
	}
	w, err := a.Week(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if want := []report.NameN{{Name: "tock", N: 3}, {Name: "Repo", N: 1}}; !reflect.DeepEqual(w.Commits, want) {
		t.Fatalf("week commits = %v", w.Commits)
	}

	// Today is live: cached for a minute, then read again.
	runs := g.runs
	if _, err := a.Day(e.Clock); err != nil || g.runs != runs {
		t.Fatalf("within a minute: %d runs, want %d (%v)", g.runs, runs, err)
	}
	e.Clock = e.Clock.Add(61 * time.Second)
	if _, err := a.Day(e.Clock); err != nil || g.runs != runs+3 {
		t.Fatalf("after a minute: %d runs, want %d (%v)", g.runs, runs+3, err)
	}
	// A past day is cached for good.
	yesterday := e.Clock.AddDate(0, 0, -1)
	if _, err := a.Day(yesterday); err != nil {
		t.Fatal(err)
	}
	runs = g.runs
	e.Clock = e.Clock.Add(time.Hour)
	if _, err := a.Day(yesterday); err != nil || g.runs != runs {
		t.Fatalf("past day reread: %d runs, want %d", g.runs, runs)
	}

	checks := a.CheckRepos()
	if len(checks) != 3 || checks[0].Repo != "/r/tock" || checks[0].Err != nil || checks[2].Repo != "/r/two" || checks[2].Err == nil {
		t.Fatalf("checks = %+v", checks)
	}
}
