package classify

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseDur(t *testing.T) {
	for in, want := range map[string]int64{"2h": 2 * 3_600_000, "45m": 45 * 60_000, "1h30": 90 * 60_000,
		"1h30m": 90 * 60_000, "0h05": 5 * 60_000} {
		if got, err := ParseDur(in); err != nil || got != want {
			t.Errorf("ParseDur(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "30", "1.5h", "h", "m", "2d", "-1h"} {
		if _, err := ParseDur(in); err == nil {
			t.Errorf("ParseDur(%q) accepted", in)
		}
	}
	for ms, want := range map[int64]string{45 * 60_000: "45m", 2 * 3_600_000: "2h", 90 * 60_000: "1h30", 65 * 60_000: "1h05"} {
		if got := FormatDur(ms); got != want {
			t.Errorf("FormatDur(%d) = %q, want %q", ms, got, want)
		}
	}
}

func TestParseGoals(t *testing.T) {
	rs, err := Parse([]byte(baseRules+`
[[goal]]
category = "code"
at_least = "2h"

[[goal]]
project = "TOCK"
at_least = "10h"
per = "week"

[[goal]]
category = "entertainment"
project = "tock"
at_most = "45m"
per = "day"
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []Goal{
		{Index: 1, Category: "Code", MS: 2 * 3_600_000},
		{Index: 2, Project: "tock", MS: 10 * 3_600_000, Week: true},
		{Index: 3, Category: "Entertainment", Project: "tock", AtMost: true, MS: 45 * 60_000},
	}
	if !reflect.DeepEqual(rs.Goals, want) {
		t.Fatalf("goals = %+v", rs.Goals)
	}
	var texts []string
	for _, g := range rs.Goals {
		texts = append(texts, g.Text())
	}
	if got := strings.Join(texts, " | "); got != "Code ≥2h/day | project tock ≥10h/week | Entertainment / tock ≤45m/day" {
		t.Fatalf("texts = %q", got)
	}
	g := rs.Goals[2]
	if !g.Matches(Label{Category: "Entertainment", Project: "tock"}) || g.Matches(Label{Category: "Entertainment"}) {
		t.Fatal("both-name goal must need both")
	}
	if !rs.Goals[1].Matches(Label{Category: "Code", Project: "tock"}) || rs.Goals[1].Matches(Label{Category: "Code"}) {
		t.Fatal("project goal matches on the project")
	}
}

func TestParseGoalErrors(t *testing.T) {
	cases := []struct{ body, want string }{
		{"[[goal]]\nat_least='2h'\n", "goal #1: sets neither category nor project"},
		{"[[goal]]\ncategory='Code'\n", "goal #1: set exactly one of at_least and at_most"},
		{"[[goal]]\ncategory='Code'\nat_least='2h'\nat_most='3h'\n", "set exactly one"},
		{"[[goal]]\ncategory='Nope'\nat_least='2h'\n", `goal #1: unknown category "Nope"`},
		{"[[goal]]\nproject='nope'\nat_least='2h'\n", `unknown project "nope"`},
		{"[[goal]]\ncategory='Code'\nat_least='2 hours'\n", `duration "2 hours"`},
		{"[[goal]]\ncategory='Code'\nat_least='0m'\n", "0m is not between 1m and 24h"},
		{"[[goal]]\ncategory='Code'\nat_least='25h'\n", "25h is not between 1m and 24h"},
		{"[[goal]]\ncategory='Code'\nat_least='169h'\nper='week'\n", "169h is not between 1m and 168h"},
		{"[[goal]]\ncategory='Code'\nat_least='2h'\nper='month'\n", `per = "month", want day or week`},
		{"[[goal]]\ncategory='Code'\nat_least='2h'\ncolour='red'\n", `unknown key "goal.colour"`},
	}
	for _, c := range cases {
		_, err := Parse([]byte(baseRules+c.body), nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: err = %v, want %q", c.body, err, c.want)
		}
	}
	if _, err := Parse([]byte(baseRules+"[[goal]]\ncategory='Code'\nat_least='25h'\nper='week'\n"), nil); err != nil {
		t.Errorf("25h a week: %v", err)
	}
	if _, err := Parse([]byte(baseRules), []byte("[[goal]]\ncategory='Code'\nat_least='2h'\n")); err == nil ||
		!strings.Contains(err.Error(), `learned.toml: unknown key "goal`) {
		t.Errorf("learned.toml goal: %v", err)
	}
}
