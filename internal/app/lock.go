package app

import "tock/internal/rulesfile"

// rules is the rule files, written only through package rulesfile.
func (a *App) rules() rulesfile.Files {
	return rulesfile.Files{Rules: a.paths.Rules, Learned: a.paths.Learned}
}

// withRulesLock runs fn holding the rule files' lock (see rulesfile.Lock).
func (a *App) withRulesLock(fn func() error) error { return a.rules().Lock(fn) }
