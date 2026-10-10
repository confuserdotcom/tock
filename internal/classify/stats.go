package classify

import "tock/internal/span"

// Rule statuses.
const (
	StatusUnused   = "unused"   // matched nothing
	StatusShadowed = "shadowed" // matched, but never decided a field
)

// RuleStat is the time one rule matched and decided over a set of views.
type RuleStat struct {
	Rule      Rule
	MatchedMS int64 // time of non-idle views the rule matches
	DecidedMS int64 // time of views where it set the category or the project
}

// Status is StatusUnused, StatusShadowed or "".
func (s RuleStat) Status() string {
	switch {
	case s.MatchedMS == 0:
		return StatusUnused
	case s.DecidedMS == 0:
		return StatusShadowed
	}
	return ""
}

// Stats measures every rule, learned rules first, over views and their
// labels (labels[i] labels views[i]). Views are grouped by their match key,
// so each rule is tested once per group.
func (rs *Ruleset) Stats(views []SpanView, labels []Label) []RuleStat {
	type group struct {
		v     *SpanView
		clean string
		ms    int64
	}
	groups := map[memoKey]*group{}
	decided := map[string]int64{}
	for i := range views {
		v := &views[i]
		if v.BundleID == span.IdleBundleID {
			continue
		}
		k, clean := keyOf(v)
		g := groups[k]
		if g == nil {
			g = &group{v: v, clean: clean}
			groups[k] = g
		}
		g.ms += v.DurMS()
		l := labels[i]
		if l.CategoryRule != "" {
			decided[l.CategoryRule] += v.DurMS()
		}
		if l.ProjectRule != "" && l.ProjectRule != l.CategoryRule {
			decided[l.ProjectRule] += v.DurMS()
		}
	}
	all := append(append([]Rule{}, rs.Learned...), rs.Rules...)
	out := make([]RuleStat, len(all))
	for i := range all {
		r := &all[i]
		out[i] = RuleStat{Rule: *r, DecidedMS: decided[r.Ref()]}
		for _, g := range groups {
			if r.matches(g.v, g.clean) {
				out[i].MatchedMS += g.ms
			}
		}
	}
	return out
}
