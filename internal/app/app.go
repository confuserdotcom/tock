// Package app is one tock session: the rules (reloaded when their files
// change) and the open database. The CLI and the TUI share it; it has no UI.
package app

import (
	"errors"
	"os"
	"time"

	"tock/internal/classify"
	"tock/internal/store"
)

// ErrNoDB means trackd has not created the database yet. tock never creates
// it.
var ErrNoDB = errors.New("no data yet: is trackd running? (make install)")

// Paths locates tock's files.
type Paths struct{ DB, Rules, Learned string }

// App is an open session. It is not safe for concurrent use.
type App struct {
	paths    Paths
	now      func() time.Time
	st       *store.Store
	rs       *classify.Ruleset
	stamp    stamp
	rulesErr error
}

// stamp identifies the versions of both rule files.
type stamp [2]struct {
	mod  time.Time
	size int64
	ok   bool
}

func stampOf(p Paths) stamp {
	var s stamp
	for i, path := range []string{p.Rules, p.Learned} {
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
	return &App{paths: p, now: now, st: st, rs: rs, stamp: st0}, nil
}

// Close closes the database.
func (a *App) Close() error { return a.st.Close() }

// Rules returns the current ruleset.
func (a *App) Rules() *classify.Ruleset { return a.rs }

// RulesErr is the error of the last failed reload, nil after a good one.
func (a *App) RulesErr() error { return a.rulesErr }

// Reload reparses the rule files when either changed (mtime or size) since
// the last load. A broken edit keeps the previous rules and is returned and
// kept in RulesErr until a later reload succeeds.
func (a *App) Reload() (changed bool, err error) {
	s := stampOf(a.paths)
	if s == a.stamp {
		return false, nil
	}
	a.stamp = s
	rs, err := classify.Load(a.paths.Rules, a.paths.Learned)
	if err != nil {
		a.rulesErr = err
		return false, err
	}
	a.rs, a.rulesErr = rs, nil
	return true, nil
}

func (a *App) nowMS() int64 { return a.now().UnixMilli() }
