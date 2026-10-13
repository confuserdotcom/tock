package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"tock/internal/apptest"
	"tock/internal/classify"
)

// oldSpans are DaySpans moved back days days, with new ids.
func oldSpans(days int) []apptest.Span {
	var out []apptest.Span
	for _, s := range apptest.DaySpans() {
		if s.End == "" {
			continue
		}
		s.ID += 1000
		s.Day = -days
		out = append(out, s)
	}
	return out
}

func labelsOn(t *testing.T, a *App, day int) []classify.Label {
	t.Helper()
	from := a.midnight(a.now()).AddDate(0, 0, day)
	_, ls, err := a.labelled(from.UnixMilli(), from.AddDate(0, 0, 1).UnixMilli(), a.nowMS())
	if err != nil {
		t.Fatal(err)
	}
	for i := range ls { // the source changes from rule to kept; the label must not
		ls[i].CategorySource, ls[i].ProjectSource, ls[i].CategoryRule, ls[i].ProjectRule = "", "", "", ""
	}
	return ls
}

func TestPruneTitles(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Seed(t, oldSpans(40)...)
	syncDir := t.TempDir()
	e.WriteConfig(t, "[privacy]\nkeep_titles = 30\n[sync]\ndir = \""+syncDir+"\"\nmachine = \"mac\"\n")
	own := filepath.Join(syncDir, "mac")
	if err := os.MkdirAll(own, 0o700); err != nil {
		t.Fatal(err)
	}
	oldFile := filepath.Join(own, "2026-08-31.jsonl")
	newFile := filepath.Join(own, "2026-10-09.jsonl")
	line := `{"s":1,"e":2,"b":"arc","a":"Arc","t":"Secret","u":"mail.com/inbox/1","c":"Communication"}` + "\n"
	for _, f := range []string{oldFile, newFile} {
		if err := os.WriteFile(f, []byte(line), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a := open(t, e)
	before := labelsOn(t, a, -40)

	dry, err := a.PruneTitles(0, true)
	if err != nil || dry.Spans != 6 || dry.Files != 1 || dry.Before.Format("2006-01-02") != "2026-09-10" {
		t.Fatalf("dry run %+v, %v", dry, err)
	}
	r, err := a.PruneTitles(0, false)
	if err != nil || r.Spans != 6 || r.Files != 1 {
		t.Fatalf("prune %+v, %v", r, err)
	}
	if after := labelsOn(t, a, -40); !reflect.DeepEqual(before, after) {
		t.Fatalf("labels changed:\n%+v\n%+v", before, after)
	}
	rows, err := a.st.Spans(0, a.midnight(a.now()).AddDate(0, 0, -30).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range rows {
		if s.Title != nil || (s.URL != nil && strings.Contains(*s.URL, "/")) {
			t.Errorf("span %d still has title %v url %v", s.ID, s.Title, s.URL)
		}
	}
	if today, _ := a.st.Spans(a.midnight(a.now()).UnixMilli(), a.nowMS()); today[0].Title == nil {
		t.Error("today's titles must stay")
	}
	if b, _ := os.ReadFile(oldFile); strings.Contains(string(b), "Secret") || !strings.Contains(string(b), `"u":"mail.com"`) ||
		!strings.Contains(string(b), `"c":"Communication"`) {
		t.Errorf("old day file: %s", b)
	}
	if b, _ := os.ReadFile(newFile); !strings.Contains(string(b), "Secret") {
		t.Errorf("recent day file pruned: %s", b)
	}
	if r, err := a.PruneTitles(0, false); err != nil || r.Spans != 0 || r.Files != 0 {
		t.Errorf("second prune %+v, %v", r, err)
	}
}

func TestPruneTitlesOff(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	if _, err := a.PruneTitles(0, false); !errors.Is(err, ErrPruneOff) {
		t.Fatalf("off: %v", err)
	}
	if r, err := a.PruneTitles(1, true); err != nil || r.Spans != 0 {
		t.Fatalf("days 1 dry run: %+v %v", r, err)
	}
}
