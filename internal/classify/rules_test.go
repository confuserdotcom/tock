package classify

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const baseRules = `
[[category]]
name = "Code"
kind = "deep"

[[category]]
name = "Entertainment"
kind = "distract"

[[project]]
name = "tock"
`

func TestParseValid(t *testing.T) {
	rs, err := Parse([]byte(baseRules+`
[[rule]]
bundle = ["com.github.wez.wezterm"]
category = "code"
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.Categories) != 2 || len(rs.Projects) != 1 || len(rs.Rules) != 1 || len(rs.Learned) != 0 {
		t.Fatalf("counts: %d categories, %d projects, %d rules, %d learned",
			len(rs.Categories), len(rs.Projects), len(rs.Rules), len(rs.Learned))
	}
	if r := rs.Rules[0]; r.Category != "Code" || r.Ref() != "rules.toml#1" {
		t.Fatalf("rule = %+v (want canonical name Code, ref rules.toml#1)", r)
	}
	if rs.Kind("CODE") != KindDeep || rs.Kind("nope") != "" {
		t.Fatal("Kind lookup wrong")
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name, rules, learned, want string
	}{
		{"syntax", baseRules + "[[rule]\n", "", "rules.toml: toml: line"},
		{"unknown key", baseRules + "[[rule]]\napp='x'\ncategory='Code'\ncolour='red'\n", "", `unknown key "rule.colour"`},
		{"bad kind", "[[category]]\nname='X'\nkind='deeep'\n", "", `kind "deeep"`},
		{"dup category", baseRules + "[[category]]\nname='code'\nkind='deep'\n", "", `duplicate category "code"`},
		{"dup project", baseRules + "[[project]]\nname='TOCK'\n", "", `duplicate project "TOCK"`},
		{"slash in name", "[[category]]\nname='A/B'\nkind='deep'\n", "", "must not contain /"},
		{"no match field", baseRules + "[[rule]]\ncategory='Code'\n", "", "rule #1: no match field"},
		{"no label", baseRules + "[[rule]]\napp='x'\n", "", "rule #1 (app='x'): sets neither category nor project"},
		{"unknown category", baseRules + "[[rule]]\nurl='^yt'\ncategory='Studdy'\n", "", `rules.toml: rule #1 (url='^yt'): unknown category "Studdy"`},
		{"unknown project", baseRules + "[[rule]]\napp='x'\nproject='atlas'\n", "", `unknown project "atlas"`},
		{"bad regex", baseRules + "[[rule]]\ntitle='('\ncategory='Code'\n", "", "bad title regex"},
		{"learned unknown category", baseRules, "[[rule]]\napp='x'\ncategory='Nope'\n", `learned.toml: rule #1 (app='x'): unknown category "Nope"`},
		{"learned declares category", baseRules, "[[category]]\nname='X'\nkind='deep'\n", `learned.toml: unknown key "category"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var learned []byte
			if c.learned != "" {
				learned = []byte(c.learned)
			}
			_, err := Parse([]byte(c.rules), learned)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

func TestLoadMissingFiles(t *testing.T) {
	dir := t.TempDir()
	rules, learned := filepath.Join(dir, RulesFile), filepath.Join(dir, LearnedFile)
	if _, err := Load(rules, learned); !errors.Is(err, ErrNoRules) {
		t.Fatalf("missing rules: err = %v, want ErrNoRules", err)
	}
	if err := os.WriteFile(rules, []byte(baseRules), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(rules, learned); err != nil {
		t.Fatalf("missing learned.toml must be fine: %v", err)
	}
}

func TestResolveNames(t *testing.T) {
	rs, err := Parse([]byte(`
[[category]]
name = "Code"
kind = "deep"
[[category]]
name = "Communication"
kind = "shallow"
[[category]]
name = "Study"
kind = "deep"
[[category]]
name = "Admin & Planning"
kind = "shallow"
[[project]]
name = "ATLAS"
[[project]]
name = "Analyse 1"
[[project]]
name = "Analyse 2"
[[project]]
name = "Old"
archived = true
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{"stu": "Study", "CODE": "Code", "adm": "Admin & Planning", "comm": "Communication"} {
		if got, err := rs.ResolveCategory(in); err != nil || got != want {
			t.Errorf("ResolveCategory(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := rs.ResolveCategory("c"); err == nil || !strings.Contains(err.Error(), "ambiguous category") {
		t.Errorf("prefix c: err = %v, want ambiguous (Code, Communication)", err)
	}
	if _, err := rs.ResolveCategory("zzz"); err == nil || !strings.Contains(err.Error(), "unknown category") {
		t.Errorf("zzz: err = %v, want unknown", err)
	}
	if got, err := rs.ResolveProject("atl"); err != nil || got != "ATLAS" {
		t.Errorf("ResolveProject(atl) = %q, %v", got, err)
	}
	if got, err := rs.ResolveProject("analyse 1"); err != nil || got != "Analyse 1" {
		t.Errorf("ResolveProject(analyse 1) = %q, %v", got, err)
	}
	if _, err := rs.ResolveProject("ana"); err == nil || !strings.Contains(err.Error(), "ambiguous project") {
		t.Errorf("ana: err = %v, want ambiguous", err)
	}
	if _, err := rs.ResolveProject("old"); err == nil || !strings.Contains(err.Error(), `project "Old" is archived`) {
		t.Errorf("archived: err = %v", err)
	}
	if _, err := rs.ResolveProject("ol"); err == nil || !strings.Contains(err.Error(), "unknown project") || strings.Contains(err.Error(), "Old") {
		t.Errorf("archived projects must not be offered by prefix: err = %v", err)
	}
	if !rs.HasProject("old") {
		t.Error("archived projects are still declared (past labels stay valid)")
	}
}
