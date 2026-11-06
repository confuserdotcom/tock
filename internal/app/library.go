package app

import "os"

// WithRulesUndo runs fn, a change to rules.toml, and journals it so Undo
// can put rules.toml back. A change that touched learned.toml too (rename,
// merge) or changed nothing is not journaled. If another process writes
// rules.toml between fn and the read after it, the entry covers that write
// too; Undo still refuses once the file differs from what it recorded.
func (a *App) WithRulesUndo(what string, fn func() error) error {
	before, rerr := os.ReadFile(a.paths.Rules)
	learned, _ := os.ReadFile(a.paths.Learned)
	if err := fn(); err != nil {
		return err
	}
	after, err := os.ReadFile(a.paths.Rules)
	if err != nil {
		return nil // the change stands; there is just nothing to journal
	}
	learnedNow, _ := os.ReadFile(a.paths.Learned)
	if string(after) == string(before) || string(learnedNow) != string(learned) {
		return nil
	}
	return a.journal(what, undoData{File: &undoFile{Path: a.paths.Rules, Existed: rerr == nil, Before: string(before), After: string(after)}})
}
