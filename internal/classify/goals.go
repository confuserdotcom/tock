package classify

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Goal is one [[goal]] table: at least (a goal) or at most (a limit) MS of
// time per day or week with the category, the project, or both.
type Goal struct {
	Index             int // 1-based, in file order
	Category, Project string
	AtMost            bool // a limit
	MS                int64
	Week              bool // per week; else per day
}

// Name is what the goal counts: "Study", "project tock" or "Code / tock".
func (g Goal) Name() string {
	switch {
	case g.Category != "" && g.Project != "":
		return g.Category + " / " + g.Project
	case g.Category != "":
		return g.Category
	}
	return "project " + g.Project
}

// Text is the goal as written, e.g. "Study ≥2h/day" or
// "Entertainment ≤45m/day".
func (g Goal) Text() string {
	op, per := "≥", "day"
	if g.AtMost {
		op = "≤"
	}
	if g.Week {
		per = "week"
	}
	return g.Name() + " " + op + FormatDur(g.MS) + "/" + per
}

// Matches reports whether a label counts towards the goal.
func (g Goal) Matches(l Label) bool {
	return (g.Category == "" || l.Category == g.Category) && (g.Project == "" || l.Project == g.Project)
}

type goalTOML struct {
	Category string `toml:"category"`
	Project  string `toml:"project"`
	AtLeast  string `toml:"at_least"`
	AtMost   string `toml:"at_most"`
	Per      string `toml:"per"`
}

var durRe = regexp.MustCompile(`^(?:(\d+)h(?:(\d+)m?)?|(\d+)m)$`)

// ParseDur reads "2h", "45m" or "1h30" as milliseconds.
func ParseDur(s string) (int64, error) {
	m := durRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("duration %q, want like 2h, 45m or 1h30", s)
	}
	n := func(x string) int64 { v, _ := strconv.ParseInt(x, 10, 64); return v }
	if m[3] != "" {
		return n(m[3]) * 60_000, nil
	}
	return n(m[1])*3_600_000 + n(m[2])*60_000, nil
}

// FormatDur writes whole minutes as ParseDur reads them: "2h", "45m", "1h30".
func FormatDur(ms int64) string {
	m := ms / 60_000
	switch {
	case m < 60:
		return fmt.Sprintf("%dm", m)
	case m%60 == 0:
		return fmt.Sprintf("%dh", m/60)
	}
	return fmt.Sprintf("%dh%02d", m/60, m%60)
}

func (rs *Ruleset) buildGoals(in []goalTOML) ([]Goal, error) {
	var out []Goal
	for i, t := range in {
		g := Goal{Index: i + 1}
		fail := func(format string, a ...any) error {
			return fmt.Errorf("%s: goal #%d: %s", RulesFile, g.Index, fmt.Sprintf(format, a...))
		}
		if t.Category == "" && t.Project == "" {
			return nil, fail("sets neither category nor project")
		}
		if t.Category != "" {
			c, ok := rs.catByLower[nameKey(t.Category)]
			if !ok {
				return nil, fail("unknown category %q", t.Category)
			}
			g.Category = c.Name
		}
		if t.Project != "" {
			p, ok := rs.projByLower[nameKey(t.Project)]
			if !ok {
				return nil, fail("unknown project %q", t.Project)
			}
			g.Project = p.Name
		}
		switch t.Per {
		case "", "day":
		case "week":
			g.Week = true
		default:
			return nil, fail("per = %q, want day or week", t.Per)
		}
		bound := t.AtLeast
		switch {
		case (t.AtLeast == "") == (t.AtMost == ""):
			return nil, fail("set exactly one of at_least and at_most")
		case t.AtMost != "":
			g.AtMost, bound = true, t.AtMost
		}
		ms, err := ParseDur(bound)
		if err != nil {
			return nil, fail("%v", err)
		}
		limit := int64(24 * 3_600_000)
		if g.Week {
			limit *= 7
		}
		if ms < 60_000 || ms > limit {
			return nil, fail("%s is not between 1m and %s", bound, FormatDur(limit))
		}
		g.MS = ms
		out = append(out, g)
	}
	return out, nil
}
