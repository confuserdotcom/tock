package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/store"
)

var errNoDB = app.ErrNoDB

func (e Env) paths() app.Paths {
	return app.Paths{DB: e.dbPath(), Rules: e.rulesPath(), Learned: e.learnedPath(),
		Config: filepath.Join(e.ConfigDir, config.File), ShellLog: e.shellLogPath(), Agents: e.AgentDir,
		AgentIndex: filepath.Join(e.StateDir, "agents.index")}
}

// openApp loads the rules and opens the database.
// When trackd has recorded but rules.toml is missing, it writes the
// starter rules (as tock init would) rather than refuse: recorded time is
// always readable. It says so once, on stderr.
func openApp(env Env) (*app.App, error) {
	a, err := openAppAsIs(env)
	if !errors.Is(err, classify.ErrNoRules) {
		return a, err
	}
	if _, serr := os.Stat(env.dbPath()); serr != nil {
		return nil, errNoDB
	}
	if werr := classify.WriteStarter(env.rulesPath()); werr != nil && !errors.Is(werr, os.ErrExist) {
		return nil, werr
	}
	fmt.Fprintf(env.Stderr, "wrote starter rules to %s (edit them, or label time in tock inbox)\n", env.rulesPath())
	return openAppAsIs(env)
}

// openAppAsIs is openApp without writing starter rules.
func openAppAsIs(env Env) (*app.App, error) {
	a, err := app.Open(env.paths(), env.Now)
	if err == nil && env.Git != nil {
		a.SetGitRunner(env.Git)
	}
	return a, err
}

// openStore opens tock.db but never creates it: an absent database means
// trackd has not run yet. For commands that work without rules.
func openStore(env Env) (*store.Store, error) {
	if _, err := os.Stat(env.dbPath()); errors.Is(err, os.ErrNotExist) {
		return nil, errNoDB
	}
	return store.Open(env.dbPath())
}
