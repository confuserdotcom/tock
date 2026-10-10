package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"tock/internal/app"
	"tock/internal/apptest"
)

func TestCommitsLines(t *testing.T) {
	e := apptest.New(t, strings.Replace(apptest.Rules, "name = \"tock\"\n", "name = \"tock\"\nrepos = [\"/r/tock\"]\n", 1))
	e.Seed(t, apptest.DaySpans()...)
	m := newModel(t, e)
	secs := []int64{apptest.At(t, "09:30", 0) / 1000, apptest.At(t, "10:00", -1) / 1000}
	m.src.do(func(a *app.App) {
		a.SetGitRunner(func(ctx context.Context, dir string, args ...string) ([]byte, error) {
			if args[0] == "config" {
				return nil, errors.New("exit status 1")
			}
			var b strings.Builder
			for _, s := range secs {
				fmt.Fprintf(&b, "%d\n", s)
			}
			return []byte(b.String()), nil
		})
	})
	m = press(t, m, "r")
	if s := screen(m); !strings.Contains(s, "commits  tock 1") {
		t.Fatalf("today lacks the commits line:\n%s", s)
	}
	m = press(t, m, "2")
	if s := screen(m); !strings.Contains(s, "Commits          tock 2") {
		t.Fatalf("week lacks the commits line:\n%s", s)
	}
}
