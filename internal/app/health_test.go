package app

import (
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/store"
)

func TestHealth(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...) // newest span start 14:50
	a := open(t, e)
	h, err := a.Health()
	if err != nil {
		t.Fatal(err)
	}
	if h.TrackdUp || h.AXTrusted != nil {
		t.Fatalf("stale data, no state: %+v", h)
	}
	e.Heartbeat(t, e.Clock.UnixMilli()-25_000)
	e.State(t, store.KeyAXTrusted, "0")
	e.State(t, store.AutomationKey("company.thebrowser.Browser"), "denied")
	e.State(t, store.AutomationKey("com.apple.Safari"), "granted")
	e.State(t, store.AutomationKey("org.example.B"), "denied")
	h, _ = a.Health()
	if !h.TrackdUp || h.AXTrusted == nil || *h.AXTrusted {
		t.Fatalf("fresh heartbeat, AX off: %+v", h)
	}
	if strings.Join(h.AutomationOff, ",") != "Arc,org.example.B" {
		t.Fatalf("AutomationOff = %v", h.AutomationOff)
	}
	e.Clock = e.Clock.Add(6 * time.Second) // 31 s since the heartbeat
	if h, _ = a.Health(); h.TrackdUp {
		t.Fatal("31 s without a heartbeat must count as down")
	}
	e.Seed(t, apptest.Span{ID: 30, Start: "15:00", End: "", Bundle: "term", App: "WezTerm", Title: "x"})
	e.Clock = apptest.Now.Add(2 * time.Second)
	if h, _ = a.Health(); !h.TrackdUp {
		t.Fatal("a span opened 2 s ago means trackd is up even before its first heartbeat")
	}
}

func TestStatus(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	if s, err := a.Status(); err != nil || s != "trackd down · 2h54" {
		t.Fatalf("down: %q %v", s, err)
	}
	e.Heartbeat(t, e.Clock.UnixMilli()-5_000)
	if s, _ := a.Status(); s != "2h54 · deep 2h14 · now unlabelled" {
		t.Fatalf("up: %q", s)
	}
}
