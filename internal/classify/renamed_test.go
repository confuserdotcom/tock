package classify

import (
	"strings"
	"testing"
)

// TestRenameLog: merges and renames are logged in rules.toml so another
// machine can follow them; CurrentName follows the log, chains included.
func TestRenameLog(t *testing.T) {
	m, err := MergeNames([]byte(dupRules), nil, NameCategory, "meeting", "Meetings")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(m.Rules), "[[renamed]]\nkind = \"category\"\nfrom = \"Meeting\"\nto = \"Meetings\"\n") {
		t.Fatalf("merge not logged:\n%s", m.Rules)
	}
	r, err := RenameName(m.Rules, nil, NameCategory, "Meetings", "Calls")
	if err != nil {
		t.Fatal(err)
	}
	rs := parseOK(t, string(r.Rules), "")
	if len(rs.Renamed) != 2 {
		t.Fatalf("renamed = %+v", rs.Renamed)
	}
	for _, c := range []struct{ in, want string }{{"Meeting", "Calls"}, {"meetings", "Calls"}, {"Calls", "Calls"}, {"Nope", ""}} {
		if got := rs.CurrentName(NameCategory, c.in); got != c.want {
			t.Errorf("CurrentName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// A cycle ends without a name; a renamed name declared again is itself.
	cyc := parseOK(t, dupRules+"\n[[renamed]]\nkind = \"category\"\nfrom = \"A\"\nto = \"B\"\n\n[[renamed]]\nkind = \"category\"\nfrom = \"B\"\nto = \"A\"\n", "")
	if got := cyc.CurrentName(NameCategory, "A"); got != "" {
		t.Errorf("cycle = %q", got)
	}
	back := parseOK(t, string(r.Rules)+"\n[[category]]\nname = \"Meeting\"\nkind = \"shallow\"\n", "")
	if got := back.CurrentName(NameCategory, "Meeting"); got != "Meeting" {
		t.Errorf("declared again = %q", got)
	}
	if _, err := Parse([]byte(dupRules+"\n[[renamed]]\nkind = \"goal\"\nfrom = \"a\"\nto = \"b\"\n"), nil); err == nil {
		t.Error("bad kind accepted")
	}
}

// TestFollowRenames rewrites a machine's learned.toml for rules taken from
// another machine that merged a category away.
func TestFollowRenames(t *testing.T) {
	m, err := MergeNames([]byte(dupRules), nil, NameCategory, "meeting", "Meetings")
	if err != nil {
		t.Fatal(err)
	}
	learned := "[[rule]]\nbundle = [\"us.zoom\"]\ncategory = \"meeting\"\n"
	if _, err := Parse(m.Rules, []byte(learned)); err == nil {
		t.Fatal("old learned should not parse with the new rules")
	}
	out, n, err := FollowRenames(m.Rules, []byte(learned))
	if err != nil || n != 1 {
		t.Fatalf("n %d err %v", n, err)
	}
	if !strings.Contains(string(out), `category = "Meetings"`) {
		t.Fatalf("learned:\n%s", out)
	}
	if _, err := Parse(m.Rules, out); err != nil {
		t.Fatal(err)
	}
}
