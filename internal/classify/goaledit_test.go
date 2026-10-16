package classify

import "testing"

const goalRules = `[[category]]
name = "Study"
kind = "deep"

[[project]]
name = "Analyse 1"
category = "Study"

[[goal]]
category = "Study"
at_least = "2h" # every day

[[goal]]
project = "Analyse 1"
at_least = "6h"
per = "week"

# the end
`

func TestSetGoal(t *testing.T) {
	rs, err := Parse([]byte(goalRules), nil)
	if err != nil {
		t.Fatal(err)
	}
	set := func(text string, g Goal) string {
		t.Helper()
		out := SetGoal(text, rs.Goals, g)
		if _, err := Parse([]byte(out), nil); err != nil {
			t.Fatalf("%v:\n%s", err, out)
		}
		return out
	}
	mod := Goal{Project: "Analyse 1", Week: true}

	mod.MS = 8 * 3_600_000
	got := set(goalRules, mod)
	want := goalRules[:len(goalRules)-len("[[goal]]\nproject = \"Analyse 1\"\nat_least = \"6h\"\nper = \"week\"\n\n# the end\n")] +
		"[[goal]]\nproject = \"Analyse 1\"\nat_least = \"8h\"\nper = \"week\"\n\n# the end\n"
	if got != want {
		t.Fatalf("replace:\n%s", got)
	}

	mod.MS = 0
	got = set(goalRules, mod)
	if want := goalRules[:len(goalRules)-len("[[goal]]\nproject = \"Analyse 1\"\nat_least = \"6h\"\nper = \"week\"\n\n# the end\n")] + "# the end\n"; got != want {
		t.Fatalf("remove:\n%s", got)
	}

	got = set(goalRules, Goal{Category: "Study", Week: true, MS: 10 * 3_600_000})
	if want := goalRules[:len(goalRules)-len("\n# the end\n")] +
		"\n[[goal]]\ncategory = \"Study\"\nat_least = \"10h\"\nper = \"week\"\n\n# the end\n"; got != want {
		t.Fatalf("add after the last goal:\n%s", got)
	}

	plain := "[[category]]\nname = \"Study\"\nkind = \"deep\"\n"
	if got := set(plain, Goal{Category: "Study", AtMost: true, MS: 45 * 60_000}); got != plain+"\n[[goal]]\ncategory = \"Study\"\nat_most = \"45m\"\n" {
		t.Fatalf("first goal:\n%s", got)
	}
	if got := SetGoal(plain, nil, Goal{Category: "Study"}); got != plain {
		t.Fatalf("removing nothing changed the text:\n%s", got)
	}
}
