// Package app is one tock session: the rules (reloaded when their files
// change) and the open database. The CLI and the TUI share it; it has no UI.
package app

import (
	"errors"
	"os"
	"time"

	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/shell"
	"tock/internal/stats"
	"tock/internal/store"
)

// ErrNoDB means trackd has not created the database yet. tock never creates
// it.
var ErrNoDB = errors.New("no data yet: is trackd running? (make install)")

// Paths locates tock's files. Config and ShellLog may name missing files;
// ShellLog "" turns shell dirs off.
type Paths struct{ DB, Rules, Learned, Config, ShellLog string }

// App is an open session. It is not safe for concurrent use.
type App struct {
	paths    Paths
	now      func() time.Time
	st       *store.Store
	rs       *classify.Ruleset
	stamp    stamp
	rulesErr error

	cfg       config.Config
	configErr error

	shell    shell.Log
	shellErr error
	home     string // the user's home, for "~/…" dirs

	past    map[string]stats.Day // stats of days before today, by date
	pastKey uint64               // overrides fingerprint the cache was built with
}

// stamp identifies the versions of the rule files and the config file.
type stamp [3]fileStamp

type fileStamp struct {
	mod  time.Time
	size int64
	ok   bool
}

func stampOf(p Paths) stamp {
	var s stamp
	for i, path := range []string{p.Rules, p.Learned, p.Config} {
		if fi, err := os.Stat(path); err == nil {
			s[i].mod, s[i].size, s[i].ok = fi.ModTime(), fi.Size(), true
		}
	}
	return s
}

// Open loads the rules, then opens the database. It fails with
// classify.ErrNoRules, a rule error, ErrNoDB or a store error.
func Open(p Paths, now func() time.Time) (*App, error) {
	st0 := stampOf(p)
	rs, err := classify.Load(p.Rules, p.Learned)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(p.DB); errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoDB
	}
	st, err := store.Open(p.DB)
	if err != nil {
		return nil, err
	}
	a := &App{paths: p, now: now, st: st, rs: rs, stamp: st0, cfg: config.Default(),
		shell: shell.Log{Path: p.ShellLog}}
	a.home, _ = os.UserHomeDir()
	a.loadConfig()
	return a, nil
}

// Close closes the database.
func (a *App) Close() error { return a.st.Close() }

// Rules returns the current ruleset.
func (a *App) Rules() *classify.Ruleset { return a.rs }

// RulesErr is the error of the last failed reload, nil after a good one.
func (a *App) RulesErr() error { return a.rulesErr }

// Config returns the current settings.
func (a *App) Config() config.Config { return a.cfg }

// ShellErr is the error of the last failed shell log read, nil after a good
// one. Views then have no dirs; it never stops tock.
func (a *App) ShellErr() error { return a.shellErr }

// ConfigErr is the error of the last failed config load, nil after a good
// one. A broken config never stops tock: the last good settings stay.
func (a *App) ConfigErr() error { return a.configErr }

func (a *App) loadConfig() {
	c, err := config.Load(a.paths.Config)
	if err != nil {
		a.configErr = err
		return
	}
	a.cfg, a.configErr = c, nil
}

// Reload reparses the rule files when either changed (mtime or size) since
// the last load, and the config file when it changed. A broken rules edit
// keeps the previous rules and is returned and kept in RulesErr until a
// later reload succeeds; a broken config edit is kept in ConfigErr.
func (a *App) Reload() (changed bool, err error) {
	s := stampOf(a.paths)
	if s[2] != a.stamp[2] {
		a.stamp[2] = s[2]
		a.loadConfig()
		a.past = nil // [shell] terminals may have changed
		changed = true
	}
	if s[0] == a.stamp[0] && s[1] == a.stamp[1] {
		return changed, nil
	}
	a.stamp[0], a.stamp[1] = s[0], s[1]
	rs, err := classify.Load(a.paths.Rules, a.paths.Learned)
	if err != nil {
		a.rulesErr = err
		return changed, err
	}
	a.rs, a.rulesErr = rs, nil
	a.past = nil
	return true, nil
}

func (a *App) nowMS() int64 { return a.now().UnixMilli() }
