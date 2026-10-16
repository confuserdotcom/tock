package classify

import (
	"strings"
	"testing"
)

const tablesRules = `# my rules

[[category]]
name = "Code"   # the main one
kind = "deep"

[[category]]
name = "Fun"
kind = "distract"

[[project]]
name = "tock"
category = "Code"

[[rule]]
app = 'wez'
category = "Code"

[[goal]]
category = "Code"
at_least = "2h"
`

func parseOK(t *testing.T, rules, learned string) *Ruleset {
	t.Helper()
	rs, err := Parse([]byte(rules), []byte(learned))
	if err != nil {
		t.Fatalf("%v\n%s", err, rules)
	}
	return rs
}

func TestAddTable(t *testing.T) {
	out := AddTable(tablesRules, NameCategory, []Field{{"name", TOMLString("Reading")}, {"kind", TOMLString("neutral")}, {"afk", TOMLString("10m")}})
	rs := parseOK(t, out, "")
	if len(rs.Categories) != 3 || rs.Categories[2].Name != "Reading" || rs.AFKWindow("reading") != 10*60_000 {
		t.Fatalf("categories %+v\n%s", rs.Categories, out)
	}
	if !strings.Contains(out, "kind = \"distract\"\n\n[[category]]\nname = \"Reading\"") ||
		!strings.Contains(out, "afk = \"10m\"\n\n[[project]]") {
		t.Fatalf("placement:\n%s", out)
	}
	// No table of the kind yet: before the first [[project]] and its comments.
	out = AddTable("# top\n\n# projects\n[[project]]\nname = 'x'\n", NameCategory, []Field{{"name", `"A"`}, {"kind", `"deep"`}})
	if out != "# top\n\n[[category]]\nname = \"A\"\nkind = \"deep\"\n\n# projects\n[[project]]\nname = 'x'\n" {
		t.Fatalf("first table:\n%q", out)
	}
	if out := AddTable("", NameCategory, []Field{{"name", `"A"`}}); out != "[[category]]\nname = \"A\"\n" {
		t.Fatalf("empty file: %q", out)
	}
}

func TestSetTableField(t *testing.T) {
	out, err := SetTableField(tablesRules, NameCategory, "code", "kind", TOMLString("shallow"))
	if err != nil || parseOK(t, out, "").Kind("Code") != KindShallow {
		t.Fatalf("%v\n%s", err, out)
	}
	out, err = SetTableField(out, NameCategory, "Code", "afk", TOMLString("15m"))
	if err != nil || !strings.Contains(out, "name = \"Code\"   # the main one\nafk = \"15m\"\nkind") {
		t.Fatalf("%v\n%s", err, out)
	}
	out, err = SetTableField(out, NameCategory, "Code", "afk", "")
	if err != nil || strings.Contains(out, "afk") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := SetTableField(tablesRules, NameCategory, "Nope", "kind", `"deep"`); err == nil {
		t.Fatal("unknown table accepted")
	}
}

func TestRenameName(t *testing.T) {
	learned := "[[rule]]\nurl = 'x'\ncategory = \"code\"\n"
	m, err := RenameName([]byte(tablesRules), []byte(learned), NameCategory, "code", "Programming")
	if err != nil {
		t.Fatal(err)
	}
	rs := parseOK(t, string(m.Rules), string(m.Learned))
	if m.From != "Code" || !rs.HasCategory("Programming") || rs.HasCategory("Code") ||
		rs.Projects[0].Category != "Programming" || rs.Goals[0].Category != "Programming" ||
		rs.Learned[0].Category != "Programming" || !strings.Contains(string(m.Rules), "# the main one") {
		t.Fatalf("%+v\n%s\n%s", rs.Categories, m.Rules, m.Learned)
	}
	for _, to := range []string{"Fun", "", "a/b", "Code"} {
		if _, err := RenameName([]byte(tablesRules), nil, NameCategory, "Code", to); err == nil {
			t.Errorf("rename to %q accepted", to)
		}
	}
	m, err = RenameName([]byte(tablesRules), nil, NameProject, "tock", "Tock app")
	if err != nil || !parseOK(t, string(m.Rules), "").HasProject("tock app") {
		t.Fatalf("%v\n%s", err, m.Rules)
	}
}

func TestRemoveName(t *testing.T) {
	out, decl, err := RemoveName([]byte(tablesRules), nil, NameCategory, "fun")
	if err != nil || decl != "Fun" || strings.Contains(string(out), "Fun") || len(parseOK(t, string(out), "").Categories) != 1 {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, _, err := RemoveName([]byte(tablesRules), nil, NameCategory, "Code"); err == nil ||
		!strings.Contains(err.Error(), "Code is named by 3 rules or goals") {
		t.Fatalf("in use: %v", err)
	}
}
