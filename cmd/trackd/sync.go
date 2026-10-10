package main

import (
	"log/slog"
	"time"

	"tock/internal/app"
)

// syncEvery is how often trackd writes its day files to the sync folder and
// reads the other machines' (v2 SYN-01). Only files: no network.
const syncEvery = time.Minute

// syncLoop runs app.Sync while config.toml sets [sync] dir. Logs hold
// counts only.
type syncLoop struct {
	open    func() (*app.App, error)
	log     *slog.Logger
	a       *app.App
	lastErr string
	on      bool
}

func (s *syncLoop) tick() {
	if s.a == nil {
		a, err := s.open()
		if err != nil {
			s.warn("sync waiting", err)
			return
		}
		s.a = a
	}
	s.a.Reload()
	if s.a.Config().Sync.Dir == "" {
		if s.on {
			s.log.Info("sync off")
		}
		s.on = false
		return
	}
	if !s.on {
		s.log.Info("sync on", "machine", s.a.Config().Sync.MachineName())
		s.on = true
	}
	r, err := s.a.Sync(app.SyncDays)
	if err != nil {
		s.warn("sync failed", err)
		return
	}
	s.lastErr = ""
	if r.Written > 0 || r.Read > 0 {
		s.log.Info("synced", "written", r.Written, "read", r.Read, "machines", len(r.Machines))
	}
}

func (s *syncLoop) warn(msg string, err error) {
	if v := msg + ": " + err.Error(); v != s.lastErr {
		s.lastErr = v
		s.log.Warn(msg, "err", err)
	}
}

func (s *syncLoop) close() {
	if s.a != nil {
		s.a.Close()
		s.a = nil
	}
}

// startSync runs the loop on its own ticker until stop is called.
func startSync(s *syncLoop, every time.Duration) (stop func()) {
	t := time.NewTicker(every)
	done, finished := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		s.tick()
		for {
			select {
			case <-t.C:
				s.tick()
			case <-done:
				return
			}
		}
	}()
	return func() {
		t.Stop()
		close(done)
		<-finished
		s.close()
	}
}
