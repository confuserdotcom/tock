package main

import (
	"errors"
	"log/slog"
	"time"

	"tock/internal/app"
	"tock/internal/snapshot"
)

// snapshotEvery is how often trackd asks whether a daily snapshot is due.
const snapshotEvery = 10 * time.Minute

// snapshotLoop takes the daily snapshot of tock.db (see app.DailySnapshot)
// and queues a notice when the database is damaged. Logs hold sizes,
// durations and errors only.
type snapshotLoop struct {
	open    func() (*app.App, error)
	log     *slog.Logger
	a       *app.App
	lastErr string
}

func (p *snapshotLoop) tick() {
	if p.a == nil {
		a, err := p.open()
		if err != nil {
			return // the focus engine already logs why the app cannot open
		}
		p.a = a
	}
	p.a.Reload()
	due, err := p.a.SnapshotDue()
	if err != nil {
		p.fail(err)
		return
	}
	if !due {
		return
	}
	start := time.Now()
	info, removed, err := p.a.DailySnapshot()
	if err != nil {
		var dmg *snapshot.DamagedError
		if errors.As(err, &dmg) {
			if nerr := p.a.NoteDamage(); nerr != nil {
				p.log.Warn("integrity notice failed", "err", nerr)
			}
		}
		p.fail(err)
		return
	}
	p.lastErr = ""
	p.log.Info("snapshot written", "bytes", info.Size, "ms", time.Since(start).Milliseconds(), "removed", removed)
}

// fail logs err once until a different one comes.
func (p *snapshotLoop) fail(err error) {
	if v := err.Error(); v != p.lastErr {
		p.lastErr = v
		p.log.Warn("snapshot failed", "err", err)
	}
}

func (p *snapshotLoop) close() {
	if p.a != nil {
		p.a.Close()
		p.a = nil
	}
}
