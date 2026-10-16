package classify

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// File names used in rule references and error messages.
const (
	RulesFile   = "rules.toml"
	LearnedFile = "learned.toml"
)

// Category kinds.
const (
	KindDeep     = "deep"
	KindShallow  = "shallow"
	KindDistract = "distract"
	KindNeutral  = "neutral"
	// KindBrowse is aimless browsing: not a distraction to nudge or block,
	// but a third of it weighs on the score like one.
	KindBrowse = "browse"
)

// Kinds lists the category kinds, most focused first.
var Kinds = []string{KindDeep, KindShallow, KindNeutral, KindBrowse, KindDistract}

// ValidKind reports whether k is a category kind.
func ValidKind(k string) bool {
	for _, x := range Kinds {
		if k == x {
			return true
		}
	}
	return false
}

// ErrNoRules is returned by Load when rules.toml does not exist.
var ErrNoRules = errors.New("no rules: run tock init")

// Category is a declared category.
type Category struct {
	Name string
	Kind string
	// AFKMS is how long time in this category may go without input before
	// it is AFK (all of it idle); 0 = DefaultAFKMS.
	AFKMS int64
}

// DefaultAFKMS is a category's AFK window when it sets none: trackd's idle
// threshold. MinAFKMS and MaxAFKMS bound the windows rules.toml may set.
const (
	DefaultAFKMS = 5 * 60_000
	MinAFKMS     = DefaultAFKMS
	MaxAFKMS     = 2 * 3_600_000
)

// Project is a declared project.
type Project struct {
	Name     string
	Archived bool
	// Category files the project under a declared category, which makes
	// it a module: time with the project and no category of its own takes
	// it, and tock finds the module by Name and Aliases in window titles
	// (unless Match is false). "" = none.
	Category string
	Aliases  []string
	Match    bool
	// Rate is the hourly rate billed in tock export; nil = none.
	Rate *float64
	// Repos are local git repositories as written (see ExpandHome).
	Repos []string
}

// MaxRate is the largest hourly rate a project may set.
const MaxRate = 100000

// Rule is one compiled [[rule]] table. Category and Project hold the declared
// (canonical) names.
type Rule struct {
	File  string // RulesFile or LearnedFile
	Index int    // 1-based position within File

	Bundle               []string
	App, Title, URL, Dir string // patterns as written
	Category             string
	Project              string

	app, title, url, dir *regexp.Regexp
}

// Ref returns "rules.toml#12".
func (r Rule) Ref() string { return fmt.Sprintf("%s#%d", r.File, r.Index) }

// Describe returns the rule's match fields, e.g. `bundle=[a] url='^x'`.
func (r Rule) Describe() string {
	var parts []string
	if len(r.Bundle) > 0 {
		parts = append(parts, "bundle=["+strings.Join(r.Bundle, ",")+"]")
	}
	for _, f := range []struct{ name, val string }{{"app", r.App}, {"title", r.Title}, {"url", r.URL}, {"dir", r.Dir}} {
		if f.val != "" {
			parts = append(parts, f.name+"='"+f.val+"'")
		}
	}
	return strings.Join(parts, " ")
}

// compile builds the regexes. Patterns are case-insensitive unless they
// start with (?-i).
func (r *Rule) compile() error {
	for _, f := range []struct {
		name string
		pat  string
		dst  **regexp.Regexp
	}{{"app", r.App, &r.app}, {"title", r.Title, &r.title}, {"url", r.URL, &r.url}, {"dir", r.Dir, &r.dir}} {
		if f.pat == "" {
			*f.dst = nil
			continue
		}
		re, err := regexp.Compile("(?i)" + f.pat)
		if err != nil {
			return fmt.Errorf("bad %s regex: %v", f.name, err)
		}
		*f.dst = re
	}
	return nil
}

// matches reports whether every match field the rule sets matches v. clean
// is Clean(*v.Title), precomputed by the caller.
func (r *Rule) matches(v *SpanView, clean string) bool {
	if len(r.Bundle) > 0 {
		ok := false
		for _, b := range r.Bundle {
			if b == v.BundleID {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if r.app != nil && !r.app.MatchString(v.App) {
		return false
	}
	if r.title != nil && (v.Title == nil || !r.title.MatchString(clean)) {
		return false
	}
	if r.url != nil && (v.URL == nil || !r.url.MatchString(*v.URL)) {
		return false
	}
	if r.dir != nil && (v.Dir == nil || !r.dir.MatchString(*v.Dir)) {
		return false
	}
	return true
}

// Ruleset is a loaded, validated pair of rule files. Learned rules are
// checked before user rules.
type Ruleset struct {
	Categories []Category
	Projects   []Project
	Learned    []Rule
	Rules      []Rule
	// CallCategory labels call time ([calls] category); "" leaves calls
	// unlabelled.
	CallCategory string
	// CalendarCategory labels meeting views the rules leave without a
	// category ([calendar] category); "" turns calendar events off.
	CalendarCategory string
	// Goals are the [[goal]] tables, in file order.
	Goals []Goal

	modules     []module    // projects with a category, in file order
	moduleKeys  []moduleKey // every name and alias of a module, for ModuleOf
	catByLower  map[string]Category
	projByLower map[string]Project

	mu   sync.Mutex
	memo map[memoKey]Label
}

type ruleTOML struct {
	Bundle   []string `toml:"bundle"`
	App      string   `toml:"app"`
	Title    string   `toml:"title"`
	URL      string   `toml:"url"`
	Dir      string   `toml:"dir"`
	Category string   `toml:"category"`
	Project  string   `toml:"project"`
}

type rulesTOML struct {
	Category []struct {
		Name string `toml:"name"`
		Kind string `toml:"kind"`
		AFK  string `toml:"afk"`
	} `toml:"category"`
	Project []struct {
		Name     string   `toml:"name"`
		Category string   `toml:"category"`
		Aliases  []string `toml:"aliases"`
		Match    *bool    `toml:"match"`
		Archived bool     `toml:"archived"`
		Rate     *float64 `toml:"rate"`
		Repos    []string `toml:"repos"`
	} `toml:"project"`
	Rule  []ruleTOML `toml:"rule"`
	Calls struct {
		Category string `toml:"category"`
	} `toml:"calls"`
	Calendar struct {
		Category string `toml:"category"`
	} `toml:"calendar"`
	Goal []goalTOML `toml:"goal"`
}

type learnedTOML struct {
	Rule []ruleTOML `toml:"rule"`
}

// decodeStrict decodes data into v and rejects unknown keys.
func decodeStrict(file string, data []byte, v any) error {
	md, err := toml.NewDecoder(bytes.NewReader(data)).Decode(v)
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return fmt.Errorf("%s: unknown key %q", file, und[0].String())
	}
	return nil
}

// Load reads and validates rulesPath and learnedPath. A missing rules file is
// ErrNoRules; a missing learned file means no learned rules.
func Load(rulesPath, learnedPath string) (*Ruleset, error) {
	rules, err := os.ReadFile(rulesPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoRules
	}
	if err != nil {
		return nil, err
	}
	learned, err := os.ReadFile(learnedPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return Parse(rules, learned)
}

// Parse validates the contents of rules.toml and learned.toml (nil when
// absent).
func Parse(rulesData, learnedData []byte) (*Ruleset, error) {
	var rf rulesTOML
	if err := decodeStrict(RulesFile, rulesData, &rf); err != nil {
		return nil, err
	}
	var lf learnedTOML
	if err := decodeStrict(LearnedFile, learnedData, &lf); err != nil {
		return nil, err
	}

	rs := &Ruleset{
		catByLower:  map[string]Category{},
		projByLower: map[string]Project{},
		memo:        map[memoKey]Label{},
	}
	for i, c := range rf.Category {
		lower := nameKey(c.Name)
		switch {
		case lower == "":
			return nil, fmt.Errorf("%s: category #%d: empty name", RulesFile, i+1)
		case strings.Contains(c.Name, "/"):
			return nil, fmt.Errorf("%s: category %q: name must not contain /", RulesFile, c.Name)
		case !ValidKind(c.Kind):
			return nil, fmt.Errorf("%s: category %q: kind %q is not deep, shallow, neutral, browse or distract", RulesFile, c.Name, c.Kind)
		}
		if _, dup := rs.catByLower[lower]; dup {
			return nil, fmt.Errorf("%s: duplicate category %q", RulesFile, c.Name)
		}
		cat := Category{Name: c.Name, Kind: c.Kind}
		if c.AFK != "" {
			ms, err := ParseDur(c.AFK)
			if err != nil || ms < MinAFKMS || ms > MaxAFKMS {
				return nil, fmt.Errorf("%s: category %q: afk %q, want 5m to 2h", RulesFile, c.Name, c.AFK)
			}
			cat.AFKMS = ms
		}
		rs.catByLower[lower] = cat
		rs.Categories = append(rs.Categories, cat)
	}
	if c := strings.TrimSpace(rf.Calls.Category); c != "" {
		cat, ok := rs.catByLower[nameKey(c)]
		if !ok {
			return nil, fmt.Errorf("%s: [calls]: unknown category %q", RulesFile, rf.Calls.Category)
		}
		rs.CallCategory = cat.Name
	}
	if c := strings.TrimSpace(rf.Calendar.Category); c != "" {
		cat, ok := rs.catByLower[nameKey(c)]
		if !ok {
			return nil, fmt.Errorf("%s: [calendar]: unknown category %q", RulesFile, rf.Calendar.Category)
		}
		rs.CalendarCategory = cat.Name
	}
	repoOwner := map[string]string{}
	for i, p := range rf.Project {
		lower := nameKey(p.Name)
		switch {
		case lower == "":
			return nil, fmt.Errorf("%s: project #%d: empty name", RulesFile, i+1)
		case strings.Contains(p.Name, "/"):
			return nil, fmt.Errorf("%s: project %q: name must not contain /", RulesFile, p.Name)
		}
		if _, dup := rs.projByLower[lower]; dup {
			return nil, fmt.Errorf("%s: duplicate project %q", RulesFile, p.Name)
		}
		proj := Project{Name: p.Name, Archived: p.Archived}
		if c := strings.TrimSpace(p.Category); c != "" {
			cat, ok := rs.catByLower[nameKey(c)]
			if !ok {
				return nil, fmt.Errorf("%s: project %q: unknown category %q", RulesFile, p.Name, p.Category)
			}
			proj.Category = cat.Name
			proj.Match = p.Match == nil || *p.Match
		}
		for _, al := range p.Aliases {
			if strings.TrimSpace(foldWords(al)) == "" {
				return nil, fmt.Errorf("%s: project %q: empty alias", RulesFile, p.Name)
			}
			proj.Aliases = append(proj.Aliases, strings.TrimSpace(al))
		}
		if proj.Category != "" && !proj.Archived {
			m := module{project: proj.Name, category: proj.Category}
			for _, k := range append([]string{proj.Name}, proj.Aliases...) {
				m.keys = append(m.keys, foldWords(k))
			}
			if !proj.Match {
				m.keys = nil // still known to the plan by name and aliases
			}
			rs.modules = append(rs.modules, m)
			for _, k := range append([]string{proj.Name}, proj.Aliases...) {
				rs.moduleKeys = append(rs.moduleKeys, moduleKey{foldWords(k), proj.Name})
			}
		}
		for _, r := range p.Repos {
			r = strings.TrimSpace(r)
			if r == "" {
				return nil, fmt.Errorf("%s: project %q: empty repo path", RulesFile, p.Name)
			}
			r = filepath.Clean(r)
			if other, dup := repoOwner[r]; dup {
				return nil, fmt.Errorf("%s: project %q: repo %q is already listed by project %q", RulesFile, p.Name, r, other)
			}
			repoOwner[r] = p.Name
			proj.Repos = append(proj.Repos, r)
		}
		if p.Rate != nil {
			if r := *p.Rate; !(r >= 0 && r <= MaxRate) {
				return nil, fmt.Errorf("%s: project %q: rate %v, want 0 to %d", RulesFile, p.Name, r, MaxRate)
			}
			proj.Rate = p.Rate
		}
		rs.projByLower[lower] = proj
		rs.Projects = append(rs.Projects, proj)
	}

	var err error
	if rs.Learned, err = rs.buildRules(LearnedFile, lf.Rule); err != nil {
		return nil, err
	}
	if rs.Rules, err = rs.buildRules(RulesFile, rf.Rule); err != nil {
		return nil, err
	}
	if rs.Goals, err = rs.buildGoals(rf.Goal); err != nil {
		return nil, err
	}
	return rs, nil
}

func (rs *Ruleset) buildRules(file string, in []ruleTOML) ([]Rule, error) {
	out := make([]Rule, 0, len(in))
	for i, t := range in {
		r := Rule{
			File: file, Index: i + 1,
			Bundle: t.Bundle, App: t.App, Title: t.Title, URL: t.URL, Dir: t.Dir,
		}
		fail := func(format string, a ...any) error {
			return fmt.Errorf("%s: rule #%d (%s): %s", file, r.Index, r.Describe(), fmt.Sprintf(format, a...))
		}
		if len(r.Bundle) == 0 && r.App == "" && r.Title == "" && r.URL == "" && r.Dir == "" {
			return nil, fmt.Errorf("%s: rule #%d: no match field (bundle, app, title, url or dir)", file, r.Index)
		}
		if t.Category == "" && t.Project == "" {
			return nil, fail("sets neither category nor project")
		}
		if t.Category != "" {
			c, ok := rs.catByLower[nameKey(t.Category)]
			if !ok {
				return nil, fail("unknown category %q", t.Category)
			}
			r.Category = c.Name
		}
		if t.Project != "" {
			p, ok := rs.projByLower[nameKey(t.Project)]
			if !ok {
				return nil, fail("unknown project %q", t.Project)
			}
			r.Project = p.Name
		}
		if err := r.compile(); err != nil {
			return nil, fail("%v", err)
		}
		out = append(out, r)
	}
	return out, nil
}

// Kind returns the kind of a declared category, or "" when undeclared.
func (rs *Ruleset) Kind(category string) string {
	return rs.catByLower[nameKey(category)].Kind
}

// AFKWindow returns a category's AFK window: its afk, else DefaultAFKMS
// (also for "" and undeclared names).
func (rs *Ruleset) AFKWindow(category string) int64 {
	if ms := rs.catByLower[nameKey(category)].AFKMS; ms > 0 {
		return ms
	}
	return DefaultAFKMS
}

// HasCategory and HasProject report whether a name is declared
// (case-insensitively).
func (rs *Ruleset) HasCategory(name string) bool {
	_, ok := rs.catByLower[nameKey(name)]
	return ok
}

func (rs *Ruleset) HasProject(name string) bool {
	_, ok := rs.projByLower[nameKey(name)]
	return ok
}

// ExpandHome replaces a leading ~ in a repo path with home.
func ExpandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

// Rate returns a declared project's hourly rate, nil when it has none.
func (rs *Ruleset) Rate(project string) *float64 {
	return rs.projByLower[nameKey(project)].Rate
}
