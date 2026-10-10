package main

import (
	"log/slog"
	"path/filepath"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/shell"
)

// defaultFocusEvery is the focus engine period.
const defaultFocusEvery = 5 * time.Second

// focusLoop runs the focus engine inside trackd: one app session on the
// same database, opened once rules.toml exists. It never stops trackd: every
// error is logged (once per distinct message) and the tick skipped.
type focusLoop struct {
	open    func() (*app.App, error)
	camera  func() bool // nil: no camera check
	mic     func() bool // nil: no microphone check
	notify  Notifier    // nil: notifications dropped
	log     *slog.Logger
	a       *app.App
	lastErr string
}

func newFocusLoop(o daemonOpts, log *slog.Logger) *focusLoop {
	paths := app.Paths{
		DB:      filepath.Join(o.DataDir, "tock.db"),
		Rules:   filepath.Join(o.ConfigDir, classify.RulesFile),
		Learned: filepath.Join(o.ConfigDir, classify.LearnedFile),
		Config:  filepath.Join(o.ConfigDir, config.File),
	}
	if o.StateDir != "" {
		paths.ShellLog = filepath.Join(o.StateDir, shell.File)
	}
	now := func() time.Time { return time.UnixMilli(o.Now()) }
	return &focusLoop{
		open:   func() (*app.App, error) { return app.Open(paths, now) },
		camera: o.Camera, mic: o.Mic, notify: o.Notifier, log: log,
	}
}

// tick runs one engine step and sends its notifications. Logs carry
// notification titles only, never bodies.
func (f *focusLoop) tick() {
	if f.a == nil {
		a, err := f.open()
		if err != nil {
			f.warn("focus engine waiting", err)
			return
		}
		f.a = a
		f.log.Info("focus engine started")
	}
	call := (f.camera != nil && f.camera()) || (f.mic != nil && f.mic())
	notes, err := f.a.FocusTick(call)
	if err != nil {
		f.warn("focus tick failed", err)
		return
	}
	f.lastErr = ""
	if !f.a.Config().Focus.Notify {
		return
	}
	// The tock menu bar app shows notifications with buttons when it runs
	// and may notify; otherwise they go out through the notifier.
	bar := len(notes) > 0 && f.a.BarReady()
	for _, n := range notes {
		if bar {
			if err := f.a.Enqueue(n); err != nil {
				f.warn("notification queue failed", err)
				continue
			}
			f.log.Info("focus notification queued", "title", n.Title)
			continue
		}
		if f.notify == nil {
			continue
		}
		if err := f.notify.Notify(n.Title, n.Body); err != nil {
			f.warn("notification failed", err)
			continue
		}
		f.log.Info("focus notification", "title", n.Title)
	}
}

func (f *focusLoop) warn(msg string, err error) {
	if s := msg + ": " + err.Error(); s != f.lastErr {
		f.lastErr = s
		f.log.Warn(msg, "err", err)
	}
}

func (f *focusLoop) close() {
	if f.a != nil {
		f.a.Close()
		f.a = nil
	}
}

// startFocus runs the loop on its own ticker until stop is called.
func startFocus(o daemonOpts, log *slog.Logger) (stop func()) {
	every := o.FocusEvery
	if every <= 0 {
		every = defaultFocusEvery
	}
	f := newFocusLoop(o, log)
	t := time.NewTicker(every)
	done, finished := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		for {
			select {
			case <-t.C:
				f.tick()
			case <-done:
				return
			}
		}
	}()
	return func() {
		t.Stop()
		close(done)
		<-finished
		f.close()
	}
}
