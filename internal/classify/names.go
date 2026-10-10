package classify

import (
	"fmt"
	"strings"
)

// ResolveCategory maps user input to a declared category name: an exact
// case-insensitive match wins, otherwise a unique case-insensitive prefix.
func (rs *Ruleset) ResolveCategory(s string) (string, error) {
	names := make([]string, len(rs.Categories))
	for i, c := range rs.Categories {
		names[i] = c.Name
	}
	return resolveName("category", s, names)
}

// ResolveProject is ResolveCategory for projects. Archived projects cannot
// be chosen for new tags; spans already labelled with them keep the label.
func (rs *Ruleset) ResolveProject(s string) (string, error) {
	names := make([]string, 0, len(rs.Projects))
	for _, p := range rs.Projects {
		if p.Archived {
			if strings.EqualFold(p.Name, strings.TrimSpace(s)) {
				return "", fmt.Errorf("project %q is archived", p.Name)
			}
			continue
		}
		names = append(names, p.Name)
	}
	return resolveName("project", s, names)
}

func resolveName(what, s string, names []string) (string, error) {
	low := strings.ToLower(strings.TrimSpace(s))
	var hits []string
	for _, n := range names {
		if strings.ToLower(n) == low {
			return n, nil
		}
		if low != "" && strings.HasPrefix(strings.ToLower(n), low) {
			hits = append(hits, n)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return "", fmt.Errorf("unknown %s %q (have: %s)", what, s, strings.Join(names, ", "))
	}
	return "", fmt.Errorf("ambiguous %s %q: %s", what, s, strings.Join(hits, ", "))
}
