package app

import (
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/classify"
)

const callRules = apptest.Rules + `
[[category]]
name = "Meeting"
kind = "shallow"

[calls]
category = "Meeting"
`

// timeline renders the day's rows as "HH:MM-HH:MM app category".
func timeline(t *testing.T, a *App) string {
	t.Helper()
	d, err := a.Day(apptest.Now)
	if err != nil {
		t.Fatal(err)
	}
	var b []string
	for _, r := range d.Timeline {
		clock := func(ms int64) string { return time.UnixMilli(ms).In(apptest.Zone).Format("15:04") }
		b = append(b, clock(r.Start)+"-"+clock(r.End)+" "+r.App+" "+r.Label.Category)
	}
	return strings.Join(b, "\n")
}

func TestCallsLabelTime(t *testing.T) {
	e := apptest.New(t, callRules)
	e.Seed(t, apptest.DaySpans()...)
	e.Call(t, "11:30", "12:00")
	e.Call(t, "14:00", "14:01") // too short
	got := timeline(t, open(t, e))
	for _, want := range []string{
		"11:26-11:30 Arc \n",
		"11:30-11:35 Arc Meeting\n",
		"11:35-11:41 Arc Meeting\n",
		"11:41-12:00 Call Meeting\n",
		"12:00-12:22 Idle \n",
	} {
		if !strings.Contains(got+"\n", want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestCallsNeedCallsTable(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Call(t, "11:30", "12:00")
	if got := timeline(t, open(t, e)); strings.Contains(got, "Call") || !strings.Contains(got, "11:41-12:22 Idle") {
		t.Fatalf("without [calls] nothing changes:\n%s", got)
	}
}

func TestFocusTickRecordsCall(t *testing.T) {
	e := apptest.New(t, callRules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	for range 25 { // two minutes of 5 s ticks
		if _, err := a.FocusTick(true); err != nil {
			t.Fatal(err)
		}
		e.Clock = e.Clock.Add(5 * 1e9)
	}
	vs, labels, err := a.labelled(apptest.At(t, "14:50", 0), e.Clock.UnixMilli(), e.Clock.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	var inCall int64
	for i, v := range vs {
		if labels[i].CategorySource == classify.SourceCall {
			inCall += v.DurMS()
		}
	}
	if inCall != CallMinMS {
		t.Fatalf("call time = %d, want %d", inCall, CallMinMS)
	}
}
