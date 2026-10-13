package main

import (
	"log/slog"
	"time"

	"tock/internal/app"
)

// pruneEvery is how often trackd deletes window titles older than
// [privacy] keep_titles.
const pruneEvery = time.Hour

// pruneLoop runs app.PruneTitles while config.toml sets [privacy]
// keep_titles. Logs hold counts only.
type pruneLoop struct {
	open    func() (*app.App, error)
	log     *slog.Logger
	a       *app.App
	lastErr string
}

func (p *pruneLoop) tick() {
	if p.a == nil {
		a, err := p.open()
		if err != nil {
			return // the focus engine already logs why the app cannot open
		}
		p.a = a
	}
	p.a.Reload()
	if p.a.Config().Privacy.KeepTitles == 0 {
		return
	}
	r, err := p.a.PruneTitles(0, false)
	if err != nil {
		if v := err.Error(); v != p.lastErr {
			p.lastErr = v
			p.log.Warn("title deletion failed", "err", err)
		}
		return
	}
	p.lastErr = ""
	if r.Spans > 0 || r.Remote > 0 || r.Files > 0 {
		p.log.Info("titles deleted", "before", r.Before.Format("2006-01-02"), "spans", r.Spans, "remote", r.Remote, "files", r.Files)
	}
}

func (p *pruneLoop) close() {
	if p.a != nil {
		p.a.Close()
		p.a = nil
	}
}
