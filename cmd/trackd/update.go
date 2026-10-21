package main

import (
	"log/slog"
	"path/filepath"

	"tock/internal/config"
	"tock/internal/store"
	"tock/internal/update"
)

// updateLoop installs the build the Mac publishes to <sync dir>/bin, once a
// tick, while [sync] update is on. It runs on Linux only (main sets
// daemonOpts.Exe there). Logs hold versions and error text only.
type updateLoop struct {
	cfgPath string // config.toml
	pinned  string // this machine's copy of the signing key
	binDir  string // where trackd and tock are installed
	arch    string
	run     update.Running
	st      *store.Store
	log     *slog.Logger
	now     func() int64
	restart func() // called once, after the files are replaced

	lastErr string // last warning, so each one shows once
	done    bool
}

func (u *updateLoop) tick() {
	if u.done {
		return
	}
	c, err := config.Load(u.cfgPath)
	if err != nil {
		u.warn("update waiting", err)
		return
	}
	if c.Sync.Dir == "" || c.Sync.NoUpdate {
		return
	}
	applied, _, err := u.st.State(update.KeyApplied)
	if err != nil {
		u.warn("update state", err)
		return
	}
	p, ok, err := update.Check(filepath.Join(c.Sync.Dir, update.BinDir), u.arch, u.pinned, u.run, applied)
	if err != nil {
		u.fail(err)
		return
	}
	if !ok {
		u.setErr("")
		return
	}
	u.log.Info("update found", "version", p.Version)
	err = update.Apply(p, u.binDir, func() error {
		return u.st.SetState(update.KeyApplied, p.Mark(), u.now())
	})
	if err != nil {
		u.fail(err)
		return
	}
	u.setErr("")
	u.log.Info("updated; restarting", "from", u.run.Version, "to", p.Version)
	u.done = true
	u.restart()
}

// fail records err as the last update error and warns about it once.
func (u *updateLoop) fail(err error) {
	u.setErr(err.Error())
	u.warn("update failed", err)
}

// setErr stores the last update error, "" when fine, if it changed.
func (u *updateLoop) setErr(msg string) {
	if msg == "" {
		u.lastErr = ""
	}
	if old, _, err := u.st.State(update.KeyError); err == nil && old == msg {
		return
	}
	if err := u.st.SetState(update.KeyError, msg, u.now()); err != nil {
		u.warn("update state", err)
	}
}

func (u *updateLoop) warn(msg string, err error) {
	if v := msg + ": " + err.Error(); v != u.lastErr {
		u.lastErr = v
		u.log.Warn(msg, "err", err)
	}
}

func (u *updateLoop) close() {}
