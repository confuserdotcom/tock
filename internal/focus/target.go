package focus

import (
	"strings"

	"tock/internal/classify"
)

// alertable reports a view whose time may alert: a distraction outside
// idle and meetings that is not marked quiet.
func alertable(in Input, i int) bool {
	v, l := in.Views[i], in.Labels[i]
	return l.Kind == classify.KindDistract && !l.Quiet && !isIdle(v) && !IsMeeting(v, in.Cfg)
}

// ruleRefOf is the rules.toml or learned.toml rule that set l's category,
// or "" for any other source (manual, call, calendar, module, kept, ...).
func ruleRefOf(l classify.Label) string {
	if l.CategorySource != classify.SourceRule && l.CategorySource != classify.SourceLearned {
		return ""
	}
	if strings.HasPrefix(l.CategoryRule, classify.RulesFile+"#") || strings.HasPrefix(l.CategoryRule, classify.LearnedFile+"#") {
		return l.CategoryRule
	}
	return ""
}

// distractTarget is what a distraction notice offers to mark quiet: the
// category of the front view when that is an alertable distraction, else
// the category with the most alertable time in the session so far (ties by
// name). ruleRef is the rule that set the category; for the fallback it is
// set only when one rule accounts for all of the category's alertable time.
// Both are "" when the session has no alertable distraction.
func distractTarget(in Input, s Session) (category, ruleRef string) {
	if n := len(in.Views); n > 0 && current(in) != nil && alertable(in, n-1) {
		l := in.Labels[n-1]
		return l.Category, ruleRefOf(l)
	}
	type tally struct {
		ms      int64
		ref     string
		oneRule bool
	}
	cats := map[string]*tally{}
	from, to := s.StartMS, min(in.Now, s.PlannedMS)
	for i, v := range in.Views {
		a, b := max(v.Start, from), min(v.End, to)
		if b <= a || !alertable(in, i) {
			continue
		}
		l := in.Labels[i]
		t := cats[l.Category]
		if t == nil {
			t = &tally{ref: ruleRefOf(l), oneRule: true}
			cats[l.Category] = t
		}
		if ref := ruleRefOf(l); ref == "" || ref != t.ref {
			t.oneRule = false
		}
		t.ms += b - a
	}
	var best string
	var bt *tally
	for name, t := range cats {
		if bt == nil || t.ms > bt.ms || (t.ms == bt.ms && name < best) {
			best, bt = name, t
		}
	}
	if bt == nil {
		return "", ""
	}
	if bt.oneRule {
		ruleRef = bt.ref
	}
	return best, ruleRef
}
