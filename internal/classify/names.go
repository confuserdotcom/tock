package classify

import (
	"errors"
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

// ResolveName resolves a category or project (NameCategory or NameProject)
// to manage it: archived projects included.
func (rs *Ruleset) ResolveName(kind, s string) (string, error) {
	if kind == NameCategory {
		return rs.ResolveCategory(s)
	}
	names := make([]string, len(rs.Projects))
	for i, p := range rs.Projects {
		names[i] = p.Name
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

// ErrNoLabel is a label string with neither a category nor a project.
var ErrNoLabel = errors.New("give a category, Category/Project or /Project")

// ParseWant resolves "Category", "Category/Project" or "/Project", each
// name by ResolveCategory or ResolveProject.
func (rs *Ruleset) ParseWant(arg string) (Want, error) {
	catPart, projPart, hasProj := strings.Cut(arg, "/")
	var w Want
	var err error
	if strings.TrimSpace(catPart) == "" && (!hasProj || strings.TrimSpace(projPart) == "") {
		return w, ErrNoLabel
	}
	if strings.TrimSpace(catPart) != "" {
		if w.Category, err = rs.ResolveCategory(catPart); err != nil {
			return w, err
		}
	}
	if hasProj && strings.TrimSpace(projPart) != "" {
		if w.Project, err = rs.ResolveProject(projPart); err != nil {
			return w, err
		}
	}
	return w, nil
}
