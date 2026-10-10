package main

import (
	"errors"
	"os"
	"path/filepath"

	"tock/internal/app"
	"tock/internal/config"
	"tock/internal/store"
)

var errNoDB = app.ErrNoDB

func (e Env) paths() app.Paths {
	return app.Paths{DB: e.dbPath(), Rules: e.rulesPath(), Learned: e.learnedPath(),
		Config: filepath.Join(e.ConfigDir, config.File)}
}

// openApp loads the rules and opens the database.
func openApp(env Env) (*app.App, error) { return app.Open(env.paths(), env.Now) }

// openStore opens tock.db but never creates it: an absent database means
// trackd has not run yet. For commands that work without rules.
func openStore(env Env) (*store.Store, error) {
	if _, err := os.Stat(env.dbPath()); errors.Is(err, os.ErrNotExist) {
		return nil, errNoDB
	}
	return store.Open(env.dbPath())
}
