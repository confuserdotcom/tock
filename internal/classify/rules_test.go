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

func TestProjectRateAndRepos(t *testing.T) {
	rs, err := Parse([]byte(baseRules+"[[project]]\nname='ATLAS'\nrate=80.5\nrepos=['~/code/atlas/', '/srv/x']\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if r := rs.Rate("atlas"); r == nil || *r != 80.5 || rs.Rate("tock") != nil || rs.Rate("nope") != nil {
		t.Fatalf("rates: %v %v", rs.Rate("atlas"), rs.Rate("tock"))
	}
	if got := rs.Projects[1].Repos; len(got) != 2 || got[0] != "~/code/atlas" || got[1] != "/srv/x" {
		t.Fatalf("repos = %q", got)
	}
	for in, want := range map[string]string{"~": "/h", "~/a/b": "/h/a/b", "/x": "/x", "~x": "~x"} {
		if got := ExpandHome(in, "/h"); got != want {
			t.Errorf("ExpandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAFKWindow(t *testing.T) {
	rs, err := Parse([]byte(baseRules+"[[category]]\nname='Reading'\nkind='deep'\nafk='12m'\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for cat, want := range map[string]int64{"reading": 12 * 60_000, "Code": DefaultAFKMS, "": DefaultAFKMS, "Nope": DefaultAFKMS} {
		if got := rs.AFKWindow(cat); got != want {
			t.Errorf("AFKWindow(%q) = %d, want %d", cat, got, want)
		}
	}
}

func TestProjectBillableAndClient(t *testing.T) {
	rs, err := Parse([]byte(baseRules+`[[project]]
name='Acme'
rate=95.5
client=' Acme Corp '
[[project]]
name='Internal'
rate=50
billable=false
[[project]]
name='Free'
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		project  string
		billable bool
		client   string
	}{
		{"acme", true, "Acme Corp"},
		{"Internal", false, ""}, // a rate, but marked non-billable
		{"Free", false, ""},     // no rate
		{"tock", false, ""},
		{"nope", false, ""},
	} {
		if got := rs.Billable(c.project); got != c.billable {
			t.Errorf("Billable(%q) = %v", c.project, got)
		}
		if got := rs.Client(c.project); got != c.client {
			t.Errorf("Client(%q) = %q", c.project, got)
		}
	}
	// The rate stays readable on a non-billable project, so turning
	// billable back on loses nothing.
	if r := rs.Rate("Internal"); r == nil || *r != 50 {
		t.Errorf("Internal rate %v", r)
	}
	if rs.BillableRate("Internal") != nil || rs.BillableRate("Free") != nil {
		t.Error("BillableRate on a non-billable project")
	}
	if r := rs.BillableRate("Acme"); r == nil || *r != 95.5 {
		t.Errorf("BillableRate(Acme) = %v", r)
	}
	if p := rs.Projects[2]; p.Billable == nil || *p.Billable || p.Client != "" {
		t.Errorf("Internal project %+v", p)
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
		{"negative rate", baseRules + "[[project]]\nname='X'\nrate=-1\n", "", `project "X": rate -1, want 0 to 100000`},
		{"huge rate", baseRules + "[[project]]\nname='X'\nrate=100001\n", "", `project "X": rate 100001`},
		{"long client", baseRules + "[[project]]\nname='X'\nclient='" + strings.Repeat("c", 101) + "'\n", "", `project "X": client is longer than 100 characters`},
		{"empty client", baseRules + "[[project]]\nname='X'\nclient=' '\n", "", `project "X": empty client`},
		{"empty repo", baseRules + "[[project]]\nname='X'\nrepos=[' ']\n", "", `project "X": empty repo path`},
		{"dup repo", baseRules + "[[project]]\nname='X'\nrepos=['~/a']\n[[project]]\nname='Y'\nrepos=['~/a/']\n", "",
			`project "Y": repo "~/a" is already listed by project "X"`},
		{"slash in name", "[[category]]\nname='A/B'\nkind='deep'\n", "", "must not contain /"},
		{"afk short", "[[category]]\nname='R'\nkind='deep'\nafk='4m'\n", "", `category "R": afk "4m", want 5m to 2h`},
		{"afk long", "[[category]]\nname='R'\nkind='deep'\nafk='3h'\n", "", `afk "3h"`},
		{"afk bad", "[[category]]\nname='R'\nkind='deep'\nafk='soon'\n", "", `afk "soon"`},
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
	for _, err := range []error{
		func() error { _, err := rs.ResolveProject("zzz"); return err }(),
		func() error { _, err := rs.ResolveCategory("zzz"); return err }(),
		func() error { _, err := rs.ResolveName(NameProject, "zzz"); return err }(),
	} {
		if !errors.Is(err, ErrUnknownName) {
			t.Errorf("unknown name: errors.Is(%v, ErrUnknownName) = false", err)
		}
	}
	if _, err := rs.ResolveProject("ana"); errors.Is(err, ErrUnknownName) {
		t.Errorf("an ambiguous name is not an unknown one: %v", err)
	}
	if !rs.HasProject("old") {
		t.Error("archived projects are still declared (past labels stay valid)")
	}
}

func TestProjectBudget(t *testing.T) {
	rs, err := Parse([]byte(baseRules+"[[project]]\nname='ATLAS'\nbudget='120h'\nbudget_since='2026-11-01'\n[[project]]\nname='Side'\nbudget='1h30'\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if p := rs.Projects[1]; p.BudgetMS != 120*3600_000 || p.BudgetSince != "2026-11-01" {
		t.Fatalf("ATLAS = %+v", p)
	}
	if p := rs.Projects[2]; p.BudgetMS != 90*60_000 || p.BudgetSince != "" {
		t.Fatalf("Side = %+v", p)
	}
	for _, bad := range []string{"budget='lots'", "budget='0m'", "budget_since='2026-11-01'", "budget='2h'\nbudget_since='Nov 1'"} {
		if _, err := Parse([]byte(baseRules+"[[project]]\nname='X'\n"+bad+"\n"), nil); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}
