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

// StatsDays is the default window of rule stats.
const StatsDays = 30

// RuleStats measures every rule over the last days days.
func (a *App) RuleStats(days int) ([]classify.RuleStat, error) {
	now := a.nowMS()
	vs, labels, err := a.labelled(now-int64(days)*24*60*60*1000, now, now)
	if err != nil {
		return nil, err
	}
	return a.rs.Stats(vs, labels), nil
}
