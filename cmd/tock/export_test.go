package main

import (
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
)

func TestExportCommand(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()[:7]...) // no open span, so later days stay empty

	code, out := te.run(t, "", "export")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, te.errb)
	}
	recs, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 8 || recs[0][0] != "id" || recs[1][9] != "Code" || recs[1][11] != "tock" {
		t.Fatalf("csv = %v", recs)
	}

	code, out = te.run(t, "", "export", "--format", "json", "--from", "2026-10-10", "--to", "2026-10-10")
	var rows []map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &rows) != nil || len(rows) != 7 {
		t.Fatalf("json exit %d, %d rows: %s", code, len(rows), out)
	}

	// Three days later the day is still in the default week; eight days
	// later it is not.
	te.now = testNow.AddDate(0, 0, 3)
	if _, out = te.run(t, "", "export"); strings.Count(out, "\n") != 8 {
		t.Errorf("3 days later: %d lines", strings.Count(out, "\n"))
	}
	te.now = testNow.AddDate(0, 0, 8)
	if _, out = te.run(t, "", "export"); strings.Count(out, "\n") != 1 {
		t.Errorf("8 days later: %q", out)
	}
	if _, out = te.run(t, "", "export", "--format", "json"); out != "[]\n" {
		t.Errorf("empty json = %q", out)
	}
}

func TestExportUsage(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	for _, args := range [][]string{
		{"export", "--format", "xml"},
		{"export", "--from", "bad"},
		{"export", "--from", "2026-10-10", "--to", "2026-10-09"},
		{"export", "extra"},
	} {
		if code, _ := te.run(t, "", args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
	if code, _ := newTestEnv(t, testRules).run(t, "", "export"); code != 1 {
		t.Errorf("no db: exit %d, want 1", code)
	}
}

func TestExportByProject(t *testing.T) {
	te := newTestEnv(t, strings.Replace(testRules, "name = \"tock\"\n", "name = \"tock\"\nrate = 60\n", 1))
	te.seed(t, daySpans()[:7]...)

	code, out := te.run(t, "", "export", "--by", "project")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, te.errb)
	}
	// Only span 1 has a project: 1h40 of tock at 60 an hour.
	if want := "project,duration_s,hours,rate,amount\ntock,6000,1.67,60,100.00\n"; out != want {
		t.Fatalf("csv =\n%s\nwant\n%s", out, want)
	}
	code, out = te.run(t, "", "export", "--by", "project", "--format", "json")
	var rows []map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &rows) != nil || len(rows) != 1 || rows[0]["amount"] != 100.0 {
		t.Fatalf("json exit %d: %s", code, out)
	}

	_, out = te.run(t, "", "export")
	recs, _ := csv.NewReader(strings.NewReader(out)).ReadAll()
	if recs[1][14] != "60" || recs[1][15] != "100.00" || recs[2][14] != "" {
		t.Fatalf("span rows = %v / %v", recs[1], recs[2])
	}
	if code, _ := te.run(t, "", "export", "--by", "app"); code != 2 {
		t.Errorf("--by app: exit %d, want 2", code)
	}
}
