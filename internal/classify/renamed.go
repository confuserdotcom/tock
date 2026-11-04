package classify

import (
	"fmt"
	"strings"
)

// Rename is one [[renamed]] table of rules.toml: a category or project
// merged or renamed away. tock rules merge and tock category/project rename
// append one, so a machine that takes these rules over sync can rename the
// same name in its own learned.toml, hand tags and synced labels.
type Rename struct {
	Kind, From, To string
}

// logRename appends the [[renamed]] table for from -> to to rules.toml text.
func logRename(text, kind, from, to string) string {
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + "\n[[renamed]]\nkind = " + tomlString(kind) + "\nfrom = " + tomlString(from) + "\nto = " + tomlString(to) + "\n"
}

// CurrentName is the declared name that name (of kind) is now: itself when
// declared, else where the rename log leads, else "".
func (rs *Ruleset) CurrentName(kind, name string) string {
	declared := func(n string) (string, bool) {
		if kind == NameCategory {
			c, ok := rs.catByLower[nameKey(n)]
			return c.Name, ok
		}
		p, ok := rs.projByLower[nameKey(n)]
		return p.Name, ok
	}
	for range len(rs.Renamed) + 1 {
		if d, ok := declared(name); ok {
			return d
		}
		next := ""
		for _, r := range rs.Renamed {
			if r.Kind == kind && nameKey(r.From) == nameKey(name) {
				next = r.To // the latest entry wins
			}
		}
		if next == "" {
			return ""
		}
		name = next
	}
	return "" // a cycle
}

// FollowRenames rewrites learned.toml's category and project values that
// rulesData no longer declares to the names its rename log leads to. It
// returns the new text and how many lines changed.
func FollowRenames(rulesData, learnedData []byte) ([]byte, int, error) {
	rs, err := Parse(rulesData, nil)
	if err != nil {
		return nil, 0, err
	}
	text := string(learnedData)
	for _, r := range rs.Renamed {
		if rs.CurrentName(r.Kind, r.From) == "" || declaredName(rs, r.Kind, r.From) {
			continue
		}
		text = renameValues(text, r.Kind, r.From, rs.CurrentName(r.Kind, r.From))
	}
	n := 0
	before, after := strings.SplitAfter(string(learnedData), "\n"), strings.SplitAfter(text, "\n")
	for i := range min(len(before), len(after)) {
		if before[i] != after[i] {
			n++
		}
	}
	return []byte(text), n, nil
}

func declaredName(rs *Ruleset, kind, name string) bool {
	if kind == NameCategory {
		return rs.HasCategory(name)
	}
	return rs.HasProject(name)
}

// parseRenamed checks the [[renamed]] tables.
func parseRenamed(in []struct{ Kind, From, To string }) ([]Rename, error) {
	out := make([]Rename, 0, len(in))
	for i, r := range in {
		r.Kind, r.From, r.To = strings.TrimSpace(r.Kind), strings.TrimSpace(r.From), strings.TrimSpace(r.To)
		if (r.Kind != NameCategory && r.Kind != NameProject) || r.From == "" || r.To == "" {
			return nil, fmt.Errorf("%s: renamed #%d: needs kind (category or project), from and to", RulesFile, i+1)
		}
		out = append(out, Rename(r))
	}
	return out, nil
}
