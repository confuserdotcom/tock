package app

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"tock/internal/classify"
)

// CategorySpec is a category to add or the fields to change: "" and 0
// leave a field as it is (Edit) or unset (Add). AFKMS < 0 drops afk.
type CategorySpec struct {
	Name, Kind string
	AFKMS      int64
}

// rewriteRules reads both rule files, lets edit rewrite them, keeps
// rules.toml as rules.toml.bak, writes learned.toml (when it changed) then
// rules.toml, and reloads.
func (a *App) rewriteRules(edit func(rules, learned []byte) ([]byte, []byte, error)) error {
	rules, err := os.ReadFile(a.paths.Rules)
	if err != nil {
		return err
	}
	learned, err := os.ReadFile(a.paths.Learned)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	newRules, newLearned, err := edit(rules, learned)
	if err != nil {
		return err
	}
	if _, err := classify.Parse(newRules, newLearned); err != nil {
		return fmt.Errorf("the rewritten rules do not parse (%v); nothing changed", err)
	}
	if err := os.WriteFile(a.paths.Rules+".bak", rules, 0o600); err != nil {
		return err
	}
	if string(newLearned) != string(learned) {
		if err := classify.WriteFile(a.paths.Learned, newLearned); err != nil {
			return err
		}
	}
	if err := classify.WriteFile(a.paths.Rules, newRules); err != nil {
		return err
	}
	_, err = a.Reload()
	return err
}

func afkValue(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return classify.TOMLString(classify.FormatDur(ms))
}

func checkCategory(c CategorySpec) error {
	if c.Kind != "" && !classify.ValidKind(c.Kind) {
		return fmt.Errorf("kind %q, want one of %s", c.Kind, strings.Join(classify.Kinds, ", "))
	}
	if c.AFKMS > 0 && (c.AFKMS < classify.MinAFKMS || c.AFKMS > classify.MaxAFKMS) {
		return fmt.Errorf("afk %s, want 5m to 2h", classify.FormatDur(c.AFKMS))
	}
	return nil
}

// AddCategory declares a new category in rules.toml.
func (a *App) AddCategory(c CategorySpec) error {
	c.Name = strings.TrimSpace(c.Name)
	switch {
	case c.Name == "" || strings.Contains(c.Name, "/"):
		return fmt.Errorf("category name %q: empty or with /", c.Name)
	case a.rs.HasCategory(c.Name):
		return fmt.Errorf("category %q exists", c.Name)
	case c.Kind == "":
		return fmt.Errorf("category %q needs a kind: %s", c.Name, strings.Join(classify.Kinds, ", "))
	}
	if err := checkCategory(c); err != nil {
		return err
	}
	fields := []classify.Field{{Key: "name", Value: classify.TOMLString(c.Name)}, {Key: "kind", Value: classify.TOMLString(c.Kind)}}
	if v := afkValue(c.AFKMS); v != "" {
		fields = append(fields, classify.Field{Key: "afk", Value: v})
	}
	return a.rewriteRules(func(rules, learned []byte) ([]byte, []byte, error) {
		return []byte(classify.AddTable(string(rules), classify.NameCategory, fields)), learned, nil
	})
}

// EditCategory changes a category's kind and AFK window (see CategorySpec);
// c.Name names it.
func (a *App) EditCategory(c CategorySpec) error {
	name, err := a.rs.ResolveCategory(c.Name)
	if err != nil {
		return err
	}
	if err := checkCategory(c); err != nil {
		return err
	}
	return a.rewriteRules(func(rules, learned []byte) ([]byte, []byte, error) {
		text := string(rules)
		var err error
		if c.Kind != "" {
			if text, err = classify.SetTableField(text, classify.NameCategory, name, "kind", classify.TOMLString(c.Kind)); err != nil {
				return nil, nil, err
			}
		}
		if c.AFKMS != 0 {
			if text, err = classify.SetTableField(text, classify.NameCategory, name, "afk", afkValue(c.AFKMS)); err != nil {
				return nil, nil, err
			}
		}
		return []byte(text), learned, nil
	})
}

// RenameName renames a category or project (classify.NameCategory or
// NameProject) everywhere: both rule files and tock.db. It returns the
// declared spelling of from.
func (a *App) RenameName(kind, from, to string) (string, error) {
	from, err := a.rs.ResolveName(kind, from)
	if err != nil {
		return "", err
	}
	var src string
	err = a.rewriteRules(func(rules, learned []byte) ([]byte, []byte, error) {
		m, err := classify.RenameName(rules, learned, kind, from, to)
		src = m.From
		return m.Rules, m.Learned, err
	})
	if err != nil {
		return "", err
	}
	return src, a.st.RenameLabel(kind, src, strings.TrimSpace(to), a.nowMS())
}

// RemoveName drops a category or project that no rule, goal or hand
// label names any more; otherwise merge it into another name.
func (a *App) RemoveName(kind, name string) (string, error) {
	name, err := a.rs.ResolveName(kind, name)
	if err != nil {
		return "", err
	}
	n, err := a.st.LabelUses(kind, name)
	if err != nil {
		return "", err
	}
	if n > 0 {
		return "", fmt.Errorf("%s is on %d labels you set by hand: merge it into another %s instead", name, n, kind)
	}
	var decl string
	err = a.rewriteRules(func(rules, learned []byte) ([]byte, []byte, error) {
		out, d, err := classify.RemoveName(rules, learned, kind, name)
		decl = d
		return out, learned, err
	})
	return decl, err
}

// TreeCategory is a category with its time and the projects filed under it.
type TreeCategory struct {
	classify.Category
	MS       int64
	Projects []TreeProject
}

// TreeProject is a project with its time.
type TreeProject struct {
	classify.Project
	MS int64
}

// LabelTree returns the declared categories in rules.toml order, each with
// the time it labelled in the last days and its projects, then (Name "")
// the projects filed under no category.
func (a *App) LabelTree(days int) ([]TreeCategory, error) {
	now := a.nowMS()
	vs, labels, err := a.labelled(now-int64(days)*24*3_600_000, now, now)
	if err != nil {
		return nil, err
	}
	cat, proj := map[string]int64{}, map[string]int64{}
	for i, v := range vs {
		cat[strings.ToLower(labels[i].Category)] += v.DurMS()
		proj[strings.ToLower(labels[i].Project)] += v.DurMS()
	}
	out := make([]TreeCategory, 0, len(a.rs.Categories)+1)
	at := map[string]int{}
	for _, c := range a.rs.Categories {
		at[strings.ToLower(c.Name)] = len(out)
		out = append(out, TreeCategory{Category: c, MS: cat[strings.ToLower(c.Name)]})
	}
	var loose []TreeProject
	for _, p := range a.rs.Projects {
		tp := TreeProject{Project: p, MS: proj[strings.ToLower(p.Name)]}
		if i, ok := at[strings.ToLower(p.Category)]; ok {
			out[i].Projects = append(out[i].Projects, tp)
		} else {
			loose = append(loose, tp)
		}
	}
	if len(loose) > 0 {
		out = append(out, TreeCategory{Projects: loose})
	}
	return out, nil
}

// ProjectSpec is a project to add, or what to change in one (Edit): nil
// leaves a field, Category "" unfiles it.
type ProjectSpec struct {
	Name     string
	Category *string
	Aliases  *[]string
	Archived *bool
}

func (a *App) checkProject(p ProjectSpec) (ProjectSpec, error) {
	if p.Category != nil && *p.Category != "" {
		c, err := a.rs.ResolveCategory(*p.Category)
		if err != nil {
			return p, err
		}
		p.Category = &c
	}
	if p.Aliases != nil {
		var clean []string
		for _, al := range *p.Aliases {
			if al = strings.TrimSpace(al); al != "" {
				clean = append(clean, al)
			}
		}
		p.Aliases = &clean
	}
	return p, nil
}

// AddProject declares a new project; with a category it is a module.
func (a *App) AddProject(p ProjectSpec) error {
	p.Name = strings.TrimSpace(p.Name)
	switch {
	case p.Name == "" || strings.Contains(p.Name, "/"):
		return fmt.Errorf("project name %q: empty or with /", p.Name)
	case a.rs.HasProject(p.Name):
		return fmt.Errorf("project %q exists", p.Name)
	}
	p, err := a.checkProject(p)
	if err != nil {
		return err
	}
	fields := []classify.Field{{Key: "name", Value: classify.TOMLString(p.Name)}}
	if p.Category != nil && *p.Category != "" {
		fields = append(fields, classify.Field{Key: "category", Value: classify.TOMLString(*p.Category)})
	}
	if p.Aliases != nil && len(*p.Aliases) > 0 {
		fields = append(fields, classify.Field{Key: "aliases", Value: classify.TOMLStrings(*p.Aliases)})
	}
	return a.rewriteRules(func(rules, learned []byte) ([]byte, []byte, error) {
		return []byte(classify.AddTable(string(rules), classify.NameProject, fields)), learned, nil
	})
}

// EditProject changes a project's category, aliases or archived flag.
func (a *App) EditProject(p ProjectSpec) error {
	name, err := a.rs.ResolveName(classify.NameProject, p.Name)
	if err != nil {
		return err
	}
	if p, err = a.checkProject(p); err != nil {
		return err
	}
	return a.rewriteRules(func(rules, learned []byte) ([]byte, []byte, error) {
		text := string(rules)
		set := func(key, value string) {
			if err == nil {
				text, err = classify.SetTableField(text, classify.NameProject, name, key, value)
			}
		}
		if p.Category != nil {
			v := ""
			if *p.Category != "" {
				v = classify.TOMLString(*p.Category)
			}
			set("category", v)
		}
		if p.Aliases != nil {
			v := ""
			if len(*p.Aliases) > 0 {
				v = classify.TOMLStrings(*p.Aliases)
			}
			set("aliases", v)
		}
		if p.Archived != nil {
			v := ""
			if *p.Archived {
				v = "true"
			}
			set("archived", v)
		}
		return []byte(text), learned, err
	})
}

// ScanModules adds a module under category for each folder in dir that no
// project names yet (by name or alias). It returns the names added.
func (a *App) ScanModules(dir, category string) ([]string, error) {
	cat, err := a.rs.ResolveCategory(category)
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(classify.ExpandHome(dir, a.home))
	if err != nil {
		return nil, err
	}
	var add []string
	for _, e := range ents {
		n := classify.NFC(strings.TrimSpace(e.Name()))
		if !e.IsDir() || n == "" || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_") || strings.Contains(n, "/") {
			continue
		}
		if a.rs.HasProject(n) || a.rs.ModuleOf(n) != "" {
			continue
		}
		add = append(add, n)
	}
	if len(add) == 0 {
		return nil, nil
	}
	err = a.rewriteRules(func(rules, learned []byte) ([]byte, []byte, error) {
		text := string(rules)
		for _, n := range add {
			text = classify.AddTable(text, classify.NameProject, []classify.Field{
				{Key: "name", Value: classify.TOMLString(n)}, {Key: "category", Value: classify.TOMLString(cat)}})
		}
		return []byte(text), learned, nil
	})
	return add, err
}
