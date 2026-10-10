package app

import (
	"fmt"

	"tock/internal/classify"
)

// RulePath is the path of classify.RulesFile or classify.LearnedFile.
func (a *App) RulePath(file string) string {
	if file == classify.LearnedFile {
		return a.paths.Learned
	}
	return a.paths.Rules
}

// MoveRule moves rule index (1-based, Rule.Index) of file one place up or
// down and reloads the rules, so labels follow at once.
func (a *App) MoveRule(file string, index int, up bool) error {
	if file != classify.RulesFile && file != classify.LearnedFile {
		return fmt.Errorf("unknown rule file %q", file)
	}
	if err := classify.MoveRule(a.RulePath(file), file, index-1, up); err != nil {
		return err
	}
	// A swap keeps the size and may keep the mtime tick: force the reload.
	a.stamp[0], a.stamp[1] = fileStamp{}, fileStamp{}
	_, err := a.Reload()
	return err
}
