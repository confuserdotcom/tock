package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"tock/internal/collect"
	"tock/internal/config"
	"tock/internal/focus"
)

// Remover takes a distraction out of the way for the focus guard.
type Remover interface {
	// CloseTab closes the front tab of the browser bundle when its host is
	// still host; false when it closed nothing.
	CloseTab(bundle, host string) (bool, error)
	// Hide hides the app; false when it was not running.
	Hide(bundle string) (bool, error)
}

type darwinRemover struct{}

func (darwinRemover) CloseTab(bundle, host string) (bool, error) {
	return collect.CloseTab(bundle, host)
}
func (darwinRemover) Hide(bundle string) (bool, error) { return collect.HideApp(bundle) }

type hyprRemover struct{}

func (hyprRemover) CloseTab(string, string) (bool, error) { return false, nil }
func (hyprRemover) Hide(class string) (bool, error)       { return collect.HyprHide(class) }

// hookTimeout bounds one hook command.
const hookTimeout = 20 * time.Second

// hookCommands are the commands for a session's start (start != nil) or
// end, in order: the Focus mode shortcut, then Slack through tock.
func hookCommands(cfg config.Config, tock string, start *focus.Session, goos string) [][]string {
	var out [][]string
	name := cfg.Focus.FocusOff
	if start != nil {
		name = cfg.Focus.FocusOn
	}
	if name != "" && goos == "darwin" {
		out = append(out, []string{"/usr/bin/shortcuts", "run", name})
	}
	if cfg.Slack.Enabled && tock != "" {
		if start != nil {
			out = append(out, []string{tock, "slack", "start", "--until", strconv.FormatInt(start.PlannedMS, 10)})
		} else {
			out = append(out, []string{tock, "slack", "stop"})
		}
	}
	return out
}

// findTock is the tock binary trackd runs for Slack: ~/.local/bin/tock (where
// make install puts it), else tock on PATH, else "".
func findTock() string {
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".local", "bin", "tock")
		if fi, err := os.Stat(p); err == nil && fi.Mode()&0o111 != 0 {
			return p
		}
	}
	if p, err := exec.LookPath("tock"); err == nil {
		return p
	}
	return ""
}

// hookWorker runs hook commands one at a time, in order, off the focus
// tick. Logs name the program and the outcome only.
type hookWorker struct {
	q   chan []string
	log *slog.Logger
	run func(ctx context.Context, argv []string) error
}

func newHookWorker(log *slog.Logger) *hookWorker {
	return &hookWorker{q: make(chan []string, 16), log: log, run: func(ctx context.Context, argv []string) error {
		return exec.CommandContext(ctx, argv[0], argv[1:]...).Run()
	}}
}

// enqueue drops the command when the queue is full rather than block the
// tick.
func (w *hookWorker) enqueue(argv []string) {
	select {
	case w.q <- argv:
	default:
		w.log.Warn("session hook dropped: queue full", "program", filepath.Base(argv[0]))
	}
}

func (w *hookWorker) loop(done <-chan struct{}) {
	for {
		select {
		case argv := <-w.q:
			w.runOne(argv)
		case <-done:
			return
		}
	}
}

func (w *hookWorker) runOne(argv []string) {
	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()
	if err := w.run(ctx, argv); err != nil {
		w.log.Warn("session hook failed", "program", filepath.Base(argv[0]), "step", hookStep(argv), "err", err)
		return
	}
	w.log.Info("session hook", "program", filepath.Base(argv[0]), "step", hookStep(argv))
}

// hookStep names a hook without its arguments (a shortcut name is the
// user's own text).
func hookStep(argv []string) string {
	if len(argv) > 2 && filepath.Base(argv[0]) == "tock" {
		return argv[1] + " " + argv[2]
	}
	return "run"
}

// blockBody is the notification after a block. It names the app, never the
// site or the title.
func blockBody(app string, closedTab bool, leftMS int64) string {
	left := (leftMS + 30_000) / 60_000
	if closedTab {
		return fmt.Sprintf("Closed a distracting tab in %s. %d min left in your session.", app, left)
	}
	return fmt.Sprintf("Hid %s. %d min left in your session.", app, left)
}

// block removes b and returns the notification to send, or false when
// nothing was removed.
func (f *focusLoop) block(b *focus.Block) (focus.Event, bool) {
	if f.remover == nil {
		return focus.Event{}, false
	}
	closed := false
	if b.Host != "" {
		ok, err := f.remover.CloseTab(b.Bundle, b.Host)
		if err != nil {
			f.warn("close tab failed", err)
		}
		closed = ok
	}
	if !closed {
		ok, err := f.remover.Hide(b.Bundle)
		if err != nil {
			f.warn("hide failed", err)
		}
		if !ok {
			return focus.Event{}, false
		}
	}
	f.log.Info("focus guard blocked a distraction", "tab", closed)
	return focus.Event{Kind: focus.Notify, Topic: focus.TopicFocus, Title: focus.TitleFocus,
		Body: blockBody(b.App, closed, b.LeftMS), OfSession: true}, true
}

// defaultRemover is the platform's Remover.
func defaultRemover() Remover {
	if runtime.GOOS == "linux" {
		return hyprRemover{}
	}
	return darwinRemover{}
}
