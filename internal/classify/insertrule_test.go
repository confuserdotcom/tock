package classify

import (
	"strings"
	"testing"
)

func TestInsertRuleTextGoesFirst(t *testing.T) {
	old := "[[category]]\nname = \"Code\"\nkind = \"deep\"\n\n# editors\n[[rule]]\napp = \"Zed\"\ncategory = \"Code\"\n"
	out, err := InsertRuleText(RulesFile, []byte(old), Rule{URL: "github\\.com", Category: "Code"}, "added in the dashboard")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "# added in the dashboard\n[[rule]]\nurl = \"github\\\\.com\"\ncategory = \"Code\"\n\n# editors\n[[rule]]") {
		t.Fatalf("new rule not first, before the old one's comment:\n%s", s)
	}
	rs, err := Parse(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.Rules) != 2 || rs.Rules[0].URL != `github\.com` {
		t.Fatalf("rules = %+v", rs.Rules)
	}
}

func TestInsertRuleTextNoRulesYet(t *testing.T) {
	old := "[[category]]\nname = \"Code\"\nkind = \"deep\"\n"
	out, err := InsertRuleText(RulesFile, []byte(old), Rule{App: "Zed", Category: "Code"}, "x")
	if err != nil {
		t.Fatal(err)
	}
	if rs, err := Parse(out, nil); err != nil || len(rs.Rules) != 1 {
		t.Fatalf("parse: %v, %+v", err, rs)
	}
}

func TestInsertRuleTextRefusesBadTOML(t *testing.T) {
	if _, err := InsertRuleText(RulesFile, []byte("[[rule]\n"), Rule{App: "Zed"}, "x"); err == nil {
		t.Fatal("want an error for a file that doesn't parse")
	}
}
