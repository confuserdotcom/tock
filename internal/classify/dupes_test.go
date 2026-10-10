package classify

import (
	"strings"
	"testing"
)

func TestSimilar(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"Admin & Planning", "admin and planning", true},
		{"Meeting", "Meetings", true},
		{"Study", "Studies", true},
		{"Code", "code ", true},
		{"Study", "Studdy", true},
		{"Entertainment", "Entertainmnet", true},
		{"Code", "Coda", false}, // short names must fold equal
		{"Study", "Social", false},
		{"Writing", "Reading", false},
	} {
		if got := similar(c.a, c.b); got != c.want {
			t.Errorf("similar(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

const dupRules = `# my rules
[[category]]
name = "Code"
kind = "deep"

# old spelling
[[category]]
name = "Meeting"
kind = "shallow"

[[category]]
name = "Meetings"
kind = "shallow"

[[project]]
name = "atlas"

[[project]]
name = "ATLAS2"
rate = 50.0

[calendar]
category = "meeting"

[[rule]]
app = 'zoom'
category = 'Meeting' # calls

[[goal]]
category = "Meeting"
at_most = "2h"
per = "day"
`

func TestDuplicates(t *testing.T) {
	rs := mustParse(t, dupRules, "")
	d := Duplicates(rs)
	if len(d) != 2 || d[0] != (Dup{NameCategory, "Meeting", "Meetings"}) || d[1] != (Dup{NameProject, "atlas", "ATLAS2"}) {
		t.Fatalf("dups = %+v", d)
	}
}

func TestMergeNames(t *testing.T) {
	learned := "[[rule]]\nbundle = [\"us.zoom\"]\ncategory = \"MEETING\"\n"
	m, err := MergeNames([]byte(dupRules), []byte(learned), NameCategory, "meeting", "Meetings")
	if err != nil {
		t.Fatal(err)
	}
	if m.From != "Meeting" || m.To != "Meetings" {
		t.Fatalf("names = %q, %q", m.From, m.To)
	}
	r, l := string(m.Rules), m.Learned
	if strings.Contains(r, "old spelling") || strings.Count(r, "[[category]]") != 2 ||
		!strings.Contains(r, "category = \"Meetings\" # calls") || strings.Contains(r, `"Meeting"`) || strings.Contains(r, `"meeting"`) {
		t.Fatalf("rules:\n%s", r)
	}
	if string(l) != "[[rule]]\nbundle = [\"us.zoom\"]\ncategory = \"Meetings\"\n" {
		t.Fatalf("learned:\n%s", l)
	}
	if _, err := MergeNames([]byte(dupRules), nil, NameProject, "ATLAS2", "atlas"); err == nil || !strings.Contains(err.Error(), "rate") {
		t.Fatalf("project with a rate merged: %v", err)
	}
	if _, err := MergeNames([]byte(dupRules), nil, NameCategory, "Code", "Nope"); err == nil {
		t.Fatal("merged into an undeclared category")
	}
	if _, err := MergeNames([]byte(dupRules), nil, NameCategory, "code", "Code"); err == nil {
		t.Fatal("merged a name into itself")
	}
}
