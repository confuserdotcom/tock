package classify

import (
	"strings"
	"testing"
)

func mustTemplates(t *testing.T, names ...string) []Template {
	t.Helper()
	ts, err := TemplatesByName(names)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestTemplatesParseOnTheirOwn(t *testing.T) {
	for _, tp := range AllTemplates() {
		if _, err := Parse(tp.Rules, nil); err != nil {
			t.Errorf("%s: %v", tp.Name, err)
		}
		if tp.Title == "" || tp.Group == "" {
			t.Errorf("%s: no title or group", tp.Name)
		}
	}
}

func TestTemplatesByNameUnknown(t *testing.T) {
	if _, err := TemplatesByName([]string{"client", "student"}); err == nil || !strings.Contains(err.Error(), "student") {
		t.Fatalf("err = %v", err)
	}
}

// The user's own Code (another kind) and their rules stay byte for byte
// at the top; only what is missing is appended, behind a dated comment.
func TestPlanTemplateKeepsYoursAppendsMissing(t *testing.T) {
	user := `[[category]]
name = "code"
kind = "shallow"

[[rule]]
title = 'a'
category = "Code"

[[rule]]
title = 'b'
category = "Code"

[[rule]]
title = 'c'
category = "Code"

[[rule]]
title = 'd'
category = "Code"

[[rule]]
title = 'e'
category = "Code"
`
	rs := mustParse(t, user, "")
	at := localMS(t, "2026-11-20T14:05:00")
	p, err := PlanTemplates(rs, []byte(user), mustTemplates(t, "client"), at, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(p.Text), user) {
		t.Fatalf("user text not kept at the top:\n%s", p.Text)
	}
	if len(p.KeptCategories) != 1 || p.KeptCategories[0] != "code" {
		t.Fatalf("kept = %v", p.KeptCategories)
	}
	if got := strings.Join(p.AddCategories, ","); got != "Meeting,Admin & Planning" {
		t.Fatalf("add categories = %s", got)
	}
	if len(p.AddRules) != 4 {
		t.Fatalf("add %d rules, want 4", len(p.AddRules))
	}
	tail := string(p.Text[len(user):])
	if !strings.Contains(tail, `# from template "client work", 2026-11-20`) {
		t.Fatalf("no header:\n%s", tail)
	}
	if strings.Count(tail, "since = 2026-11-20T14:05:00") != 4 {
		t.Fatalf("rules not dated:\n%s", tail)
	}
	got, err := Parse(p.Text, nil)
	if err != nil {
		t.Fatalf("result does not parse: %v", err)
	}
	if got.Kind("Code") != "shallow" {
		t.Fatalf("your Code kind changed to %q", got.Kind("Code"))
	}
}

// A second plan over the result has nothing to do.
func TestPlanTemplateTwiceNothing(t *testing.T) {
	rs := mustParse(t, string(Starter), "")
	ts := mustTemplates(t, "client", "own")
	p, err := PlanTemplates(rs, Starter, ts, localMS(t, "2026-11-20T14:05:00"), false)
	if err != nil {
		t.Fatal(err)
	}
	if p.Empty() {
		t.Fatal("first plan empty")
	}
	rs2 := mustParse(t, string(p.Text), "")
	p2, err := PlanTemplates(rs2, p.Text, ts, localMS(t, "2026-11-21T09:00:00"), false)
	if err != nil {
		t.Fatal(err)
	}
	if !p2.Empty() || string(p2.Text) != string(p.Text) {
		t.Fatalf("second plan not empty: add cats %v rules %d", p2.AddCategories, len(p2.AddRules))
	}
	if len(p2.HaveRules) == 0 {
		t.Fatal("second plan lists no rules already there")
	}
}

// Two templates sharing a rule add it once.
func TestPlanTemplateSharedRuleOnce(t *testing.T) {
	rs := mustParse(t, baseRules, "")
	p, err := PlanTemplates(rs, []byte(baseRules), mustTemplates(t, "client", "own"), localMS(t, "2026-11-20T14:05:00"), true)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range p.AddRules {
		if r.Category == "Code" && len(r.Bundle) > 0 {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("editor rule added %d times", n)
	}
	if strings.Contains(string(p.Text), "since =") {
		t.Fatal("past apply (since 0) wrote since")
	}
}

// Re-applying with past over a dated rule never rewrites it; the plan says
// how to apply it to the past.
func TestPlanTemplatePastOverDatedRule(t *testing.T) {
	rs := mustParse(t, baseRules, "")
	ts := mustTemplates(t, "own")
	p, err := PlanTemplates(rs, []byte(baseRules), ts, localMS(t, "2026-11-20T14:05:00"), false)
	if err != nil {
		t.Fatal(err)
	}
	rs2 := mustParse(t, string(p.Text), "")
	p2, err := PlanTemplates(rs2, p.Text, ts, localMS(t, "2026-11-22T09:00:00"), true)
	if err != nil {
		t.Fatal(err)
	}
	if !p2.Empty() {
		t.Fatal("past re-apply wants to change something")
	}
	dated := 0
	for _, h := range p2.HaveRules {
		if h.SinceMS != 0 {
			dated++
		}
	}
	if dated != len(p2.HaveRules) || dated == 0 {
		t.Fatalf("have %d rules, %d dated", len(p2.HaveRules), dated)
	}
}

func TestPlanTemplateBrokenRulesFails(t *testing.T) {
	rs := mustParse(t, baseRules, "")
	if _, err := PlanTemplates(rs, []byte("[[rule]\n"), mustTemplates(t, "own"), 0, false); err == nil {
		t.Fatal("broken text planned")
	}
}
