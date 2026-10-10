package app

import (
	"errors"
	"os"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/classify"
	"tock/internal/report"
)

func open(t *testing.T, e *apptest.Env) *App {
	t.Helper()
	a, err := Open(Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func TestOpenErrors(t *testing.T) {
	e := apptest.New(t, "")
	p := Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned}
	if _, err := Open(p, e.NowFunc()); !errors.Is(err, classify.ErrNoRules) {
		t.Fatalf("no rules: %v", err)
	}
	e.WriteRules(t, apptest.Rules)
	if _, err := Open(p, e.NowFunc()); !errors.Is(err, ErrNoDB) {
		t.Fatalf("no db: %v", err)
	}
	if _, err := os.Stat(e.DB); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Open must never create the database")
	}
}

func TestDayAndWeek(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Seed(t, apptest.Span{ID: 9, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "tock │ y", Day: -2})
	a := open(t, e)
	d, err := a.Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if report.Dur(d.TrackedMS) != "2h54" || report.Dur(d.IdleMS) != "41m" {
		t.Fatalf("day tracked %s idle %s", report.Dur(d.TrackedMS), report.Dur(d.IdleMS))
	}
	if last := d.Timeline[len(d.Timeline)-1]; !last.Open || last.App != "Mail" {
		t.Fatalf("last row must be the open Mail span: %+v", last)
	}
	w, err := a.Week(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if !w.Monday.Equal(time.Date(2026, 10, 5, 0, 0, 0, 0, apptest.Zone)) {
		t.Fatalf("Monday = %v", w.Monday)
	}
	if report.Dur(w.Days[3].TrackedMS) != "1h00" || report.Dur(w.Days[5].TrackedMS) != "2h54" || !w.Days[6].Future {
		t.Fatalf("week days = %+v", w.Days)
	}
}

func TestInboxAndKeySpans(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	items, err := a.Inbox(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].Key != "Arc · chatgpt.com/c" {
		t.Fatalf("inbox = %+v", items)
	}
	ids, ms, err := a.KeySpans("Arc · chatgpt.com/c")
	if err != nil || len(ids) != 2 || report.Dur(ms) != "12m" {
		t.Fatalf("KeySpans = %v %s %v", ids, report.Dur(ms), err)
	}
}

func TestReloadOnEditAndBrokenEdit(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	if changed, err := a.Reload(); changed || err != nil {
		t.Fatalf("no edit: changed %v err %v", changed, err)
	}
	e.WriteRules(t, apptest.Rules+"\n[[rule]]\napp = '^Mail$'\ncategory = \"Communication\"\n")
	if changed, err := a.Reload(); !changed || err != nil {
		t.Fatalf("edit: changed %v err %v", changed, err)
	}
	d, _ := a.Day(e.Clock)
	if d.ByKind[classify.KindShallow] != 10*60_000 {
		t.Fatalf("edited rule not applied: %v", d.ByKind)
	}
	e.WriteRules(t, apptest.Rules+"\n[[rule]]\napp = '('\ncategory = \"Code\"\n")
	if _, err := a.Reload(); err == nil || a.RulesErr() == nil {
		t.Fatal("broken edit must report an error")
	}
	if d, _ = a.Day(e.Clock); d.ByKind[classify.KindShallow] != 10*60_000 {
		t.Fatal("broken edit must keep the last good rules")
	}
	if changed, err := a.Reload(); changed || err != nil {
		t.Fatalf("unchanged broken file is not reparsed: %v %v", changed, err)
	}
	if a.RulesErr() == nil {
		t.Fatal("RulesErr is kept until a good reload")
	}
	e.WriteRules(t, apptest.Rules)
	if changed, err := a.Reload(); !changed || err != nil || a.RulesErr() != nil {
		t.Fatalf("fixed file: changed %v err %v rulesErr %v", changed, err, a.RulesErr())
	}
}

// A real clock moves between reads; the open row must still be found.
func TestOpenRowWithMovingClock(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Heartbeat(t, e.Clock.UnixMilli())
	a, err := Open(Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned}, func() time.Time {
		e.Clock = e.Clock.Add(time.Millisecond)
		return e.Clock
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	d, err := a.Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if last := d.Timeline[len(d.Timeline)-1]; !last.Open {
		t.Fatalf("last row must be open: %+v", last)
	}
	if s, _ := a.Status(); s != "2h54 · deep 2h14 · now unlabelled" {
		t.Fatalf("status = %q", s)
	}
}
