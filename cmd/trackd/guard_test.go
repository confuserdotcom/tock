package main

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"tock/internal/apptest"
	"tock/internal/config"
	"tock/internal/focus"
)

type fakeRemover struct {
	closeOK, hideOK bool
	calls           []string
}

func (r *fakeRemover) CloseTab(bundle, host string) (bool, error) {
	r.calls = append(r.calls, "close "+bundle+" "+host)
	return r.closeOK, nil
}

func (r *fakeRemover) Hide(bundle string) (bool, error) {
	r.calls = append(r.calls, "hide "+bundle)
	return r.hideOK, nil
}

func TestFocusLoopBlocks(t *testing.T) {
	for _, c := range []struct {
		closeOK, hideOK bool
		calls           []string
		note            string
	}{
		{true, true, []string{"close arc www.youtube.com"}, "tock · focus|Closed a distracting tab in Arc. 15 min left in your session."},
		{false, true, []string{"close arc www.youtube.com", "hide arc"}, "tock · focus|Hid Arc. 15 min left in your session."},
		{false, false, []string{"close arc www.youtube.com", "hide arc"}, ""},
	} {
		e := distractedEnv(t, apptest.Rules)
		e.WriteConfig(t, "[focus]\nblock = true\ndistract_grace = 3600\n")
		n := &recNotifier{}
		f, logb := testLoop(t, e, n, nil)
		r := &fakeRemover{closeOK: c.closeOK, hideOK: c.hideOK}
		f.remover = r
		var hooks [][]string
		f.hooks = func(argv []string) { hooks = append(hooks, argv) }
		f.tick()
		if !reflect.DeepEqual(r.calls, c.calls) {
			t.Errorf("calls = %q, want %q", r.calls, c.calls)
		}
		var want []string
		if c.note != "" {
			want = []string{c.note}
		}
		if !reflect.DeepEqual(n.got, want) {
			t.Errorf("notes = %q, want %q", n.got, want)
		}
		if len(hooks) != 0 { // no focus_on and no Slack configured
			t.Errorf("hooks = %q", hooks)
		}
		if strings.Contains(logb.String(), "youtube") {
			t.Errorf("log leaks the host: %s", logb.String())
		}
	}
}

func TestHookCommands(t *testing.T) {
	cfg := config.Default()
	s := &focus.Session{PlannedMS: 1234}
	if got := hookCommands(cfg, "/bin/tock", s, "darwin"); len(got) != 0 {
		t.Fatalf("nothing configured: %q", got)
	}
	cfg.Focus.FocusOn, cfg.Focus.FocusOff = "Work", "Off"
	cfg.Slack.Enabled = true
	want := [][]string{{"/usr/bin/shortcuts", "run", "Work"}, {"/bin/tock", "slack", "start", "--until", "1234"}}
	if got := hookCommands(cfg, "/bin/tock", s, "darwin"); !reflect.DeepEqual(got, want) {
		t.Fatalf("start = %q", got)
	}
	want = [][]string{{"/usr/bin/shortcuts", "run", "Off"}, {"/bin/tock", "slack", "stop"}}
	if got := hookCommands(cfg, "/bin/tock", nil, "darwin"); !reflect.DeepEqual(got, want) {
		t.Fatalf("stop = %q", got)
	}
	if got := hookCommands(cfg, "", nil, "linux"); len(got) != 0 {
		t.Fatalf("linux without tock: %q", got)
	}
}

func TestFocusLoopRunsHooksOnce(t *testing.T) {
	e := distractedEnv(t, apptest.Rules)
	e.WriteConfig(t, "[focus]\nfocus_on = \"Work\"\nfocus_off = \"Off\"\n")
	f, _ := testLoop(t, e, &recNotifier{}, nil)
	f.goos = "darwin"
	var hooks [][]string
	f.hooks = func(argv []string) { hooks = append(hooks, argv) }
	f.tick()
	f.tick()
	if want := [][]string{{"/usr/bin/shortcuts", "run", "Work"}}; !reflect.DeepEqual(hooks, want) {
		t.Fatalf("hooks = %q", hooks)
	}
}

func TestHookWorkerLogsNoArgs(t *testing.T) {
	var logb strings.Builder
	w := &hookWorker{q: make(chan []string, 1), log: slog.New(slog.NewTextHandler(&logb, nil)),
		run: func(context.Context, []string) error { return errors.New("exit status 1") }}
	w.runOne([]string{"/usr/bin/shortcuts", "run", "Secret Name"})
	w.runOne([]string{"/x/tock", "slack", "start", "--until", "5"})
	out := logb.String()
	if !strings.Contains(out, "session hook failed") || !strings.Contains(out, `step="slack start"`) || strings.Contains(out, "Secret") {
		t.Fatalf("log = %s", out)
	}
	w.enqueue([]string{"a"})
	w.enqueue([]string{"b"}) // full: dropped, never blocks
	if len(w.q) != 1 {
		t.Fatalf("queue = %d", len(w.q))
	}
}
