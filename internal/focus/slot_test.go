package focus

import (
	"strings"
	"testing"

	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/span"
)

const reviewKey = "plan:2026-10-10T14:00:x"

func reviewSlot() Slot {
	return Slot{Key: reviewKey, Kind: SlotPlan, StartMS: at(14, 0, 0), EndMS: at(14, 50, 0), Title: "Review"}
}

// slotInput is a tick at now with the default plan config and the review slot.
func slotInput(now int64) Input {
	in := input(now)
	in.Plan = config.DefaultPlan()
	in.Slots = []Slot{reviewSlot()}
	return in
}

func TestStepSlotStarting(t *testing.T) {
	in := slotInput(at(14, 0, 30))
	evs := only(Step(in), SlotDue)
	if len(evs) != 1 {
		t.Fatalf("events = %+v, want exactly one SlotDue", evs)
	}
	e := evs[0]
	if e.Topic != TopicStarting || e.Title != "Starting now" || e.Body != "14:00–14:50 Review" || e.SlotKind != SlotStarting || e.Slot != reviewSlot() {
		t.Fatalf("event = %+v", e)
	}
	if !SlotsDue(in) {
		t.Error("SlotsDue = false at 14:00:30")
	}

	late := slotInput(at(14, 2, 0))
	if got := only(Step(late), SlotDue); len(got) != 0 || SlotsDue(late) {
		t.Errorf("at 14:02: events %+v, SlotsDue %v, want none", got, SlotsDue(late))
	}

	sent := slotInput(at(14, 0, 30))
	sent.SlotSent = map[string]bool{SlotSentKey(reviewKey, SlotStarting): true}
	if got := only(Step(sent), SlotDue); len(got) != 0 {
		t.Errorf("already sent: %+v, want none", got)
	}

	off := slotInput(at(14, 0, 30))
	off.Plan.Notify = false
	if got := only(Step(off), SlotDue); len(got) != 0 || SlotsDue(off) {
		t.Errorf("notify off: events %+v, SlotsDue %v", got, SlotsDue(off))
	}

	call := slotInput(at(14, 0, 30))
	call.Call = true
	if got := only(Step(call), SlotDue); len(got) != 1 {
		t.Errorf("during a call: %+v, want the one event (not deferred)", got)
	}
}

func TestSlotName(t *testing.T) {
	for _, tt := range []struct{ title, want string }{
		{"Review https://zoom.us/j/1 notes", "Review notes"},
		{"http://x.y", "(no title)"},
		{"", "(no title)"},
		{"   ", "(no title)"},
		{"Review  the   plan", "Review the plan"},
	} {
		if got := SlotName(Slot{Title: tt.title}, 40); got != tt.want {
			t.Errorf("SlotName(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
	long := SlotName(Slot{Title: strings.Repeat("a", 50)}, 40)
	if long != strings.Repeat("a", 39)+"…" {
		t.Errorf("long title = %q (%d runes), want 39 a + …", long, len([]rune(long)))
	}
}

func TestStepSlotUpcoming(t *testing.T) {
	for _, tt := range []struct {
		now  int64
		body string // "" = no event
	}{
		{at(13, 55, 0), "14:00 Review · in 5 min"},
		{at(13, 57, 10), "14:00 Review · in 3 min"},
		{at(13, 59, 30), "14:00 Review · in 1 min"},
		{at(13, 54, 59), ""},
	} {
		in := slotInput(tt.now)
		evs := only(Step(in), SlotDue)
		if tt.body == "" {
			if len(evs) != 0 || SlotsDue(in) {
				t.Errorf("now %d: events %+v, SlotsDue %v, want none", tt.now, evs, SlotsDue(in))
			}
			continue
		}
		if len(evs) != 1 || evs[0].Topic != TopicUpcoming || evs[0].Title != "Up next" || evs[0].Body != tt.body || evs[0].SlotKind != SlotUpcoming || !SlotsDue(in) {
			t.Errorf("now %d: events %+v, want one up-next %q", tt.now, evs, tt.body)
		}
	}

	both := slotInput(at(14, 0, 0))
	if evs := only(Step(both), SlotDue); len(evs) != 1 || evs[0].SlotKind != SlotStarting {
		t.Errorf("at 14:00: %+v, want only the starting event", evs)
	}

	sent := slotInput(at(13, 56, 0))
	sent.SlotSent = map[string]bool{SlotSentKey(reviewKey, SlotUpcoming): true}
	if evs := only(Step(sent), SlotDue); len(evs) != 0 {
		t.Errorf("up-next already sent: %+v", evs)
	}

	none := slotInput(at(13, 56, 0))
	none.Plan.NotifyBeforeMS = 0
	if evs := only(Step(none), SlotDue); len(evs) != 0 || SlotsDue(none) {
		t.Errorf("notify_before 0: %+v, SlotsDue %v", evs, SlotsDue(none))
	}
	none.Now = at(14, 0, 0)
	if evs := only(Step(none), SlotDue); len(evs) != 1 || evs[0].SlotKind != SlotStarting {
		t.Errorf("notify_before 0 at 14:00: %+v, want the starting event", evs)
	}

	off := slotInput(at(13, 56, 0))
	off.Plan.Notify = false
	if evs := only(Step(off), SlotDue); len(evs) != 0 {
		t.Errorf("notify off: %+v", evs)
	}
}

func TestSlotFirstSeen(t *testing.T) {
	late := slotInput(at(13, 59, 20))
	late.Slots[0].SeenMS = at(13, 59, 10)
	if evs := only(Step(late), SlotDue); len(evs) != 0 {
		t.Errorf("seen 50 s before start: %+v, want no up-next", evs)
	}
	late.Now = at(14, 0, 10)
	if evs := only(Step(late), SlotDue); len(evs) != 1 || evs[0].SlotKind != SlotStarting {
		t.Errorf("seen late, at 14:00:10: %+v, want the starting event", evs)
	}
	early := slotInput(at(13, 57, 5))
	early.Slots[0].SeenMS = at(13, 57, 0)
	if evs := only(Step(early), SlotDue); len(evs) != 1 || evs[0].Body != "14:00 Review · in 3 min" {
		t.Errorf("seen 3 min before: %+v, want up-next in 3 min", evs)
	}
}

func standupSlot(calendar string) Slot {
	return Slot{Key: "event:1:x", Kind: SlotEvent, StartMS: at(14, 30, 0), EndMS: at(15, 0, 0), Title: "Standup", Calendar: calendar}
}

func slotBody(t *testing.T, now int64, s Slot) string {
	t.Helper()
	in := input(now)
	in.Plan = config.DefaultPlan()
	in.Slots = []Slot{s}
	evs := only(Step(in), SlotDue)
	if len(evs) != 1 {
		t.Fatalf("events = %+v, want exactly one SlotDue", evs)
	}
	return evs[0].Body
}

func TestSlotEventBodies(t *testing.T) {
	for _, tt := range []struct {
		name string
		now  int64
		slot Slot
		want string
	}{
		{"upcoming", at(14, 25, 0), standupSlot("Work"), "14:30 Standup (Work) · in 5 min"},
		{"starting", at(14, 30, 10), standupSlot("Work"), "14:30–15:00 Standup (Work)"},
		{"upcoming no calendar", at(14, 25, 0), standupSlot(""), "14:30 Standup · in 5 min"},
		{"starting no calendar", at(14, 30, 10), standupSlot(""), "14:30–15:00 Standup"},
		{"long calendar", at(14, 30, 10), standupSlot(strings.Repeat("c", 30)), "14:30–15:00 Standup (" + strings.Repeat("c", 23) + "…)"},
	} {
		s := tt.slot
		s.SeenMS = 0
		if got := slotBody(t, tt.now, s); got != tt.want {
			t.Errorf("%s: body = %q, want %q", tt.name, got, tt.want)
		}
	}
	plan := reviewSlot()
	plan.Calendar = "Ignored"
	if got := slotBody(t, at(14, 0, 30), plan); got != "14:00–14:50 Review" {
		t.Errorf("plan slot body = %q, want it without a calendar", got)
	}
}

func TestSlotOwner(t *testing.T) {
	now := at(14, 0, 0)
	for _, tt := range []struct {
		name   string
		local  int64
		others []int64
		want   bool
	}{
		{"no other machine", at(13, 0, 0), nil, true},
		{"other older", at(13, 50, 0), []int64{at(13, 40, 0)}, true},
		{"other newer, 5 min old", at(13, 54, 0), []int64{at(13, 55, 0)}, false},
		{"other newer but stale", at(13, 30, 0), []int64{at(13, 44, 0)}, true},
		{"tie", at(13, 50, 0), []int64{at(13, 50, 0)}, true},
		{"other far in the future", at(13, 50, 0), []int64{at(14, 20, 0)}, true},
		{"other slightly ahead of now", at(13, 59, 0), []int64{at(14, 2, 0)}, false},
		{"one stale and one older", at(13, 50, 0), []int64{at(13, 0, 0), at(13, 40, 0)}, true},
		{"one stale and one newer", at(13, 50, 0), []int64{at(13, 0, 0), at(13, 55, 0)}, false},
		{"no local input at all", 0, []int64{at(13, 55, 0)}, false},
	} {
		if got := SlotOwner(tt.local, now, tt.others); got != tt.want {
			t.Errorf("%s: SlotOwner = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestLocalInputEnd(t *testing.T) {
	if got := LocalInputEnd(nil); got != 0 {
		t.Errorf("no views: %d, want 0", got)
	}
	views := []classify.SpanView{
		{ID: 1, Start: at(13, 0, 0), End: at(13, 40, 0), BundleID: "app"},
		{ID: 2, Start: at(13, 40, 0), End: at(14, 0, 0), BundleID: span.IdleBundleID},
		{ID: -(classify.RemoteIDBase + 1), Start: at(13, 40, 0), End: at(13, 59, 0), BundleID: "app"},
		{ID: -3, Start: at(13, 41, 0), End: at(13, 50, 0), BundleID: "tock.away"},
	}
	if got := LocalInputEnd(views); got != at(13, 40, 0) {
		t.Errorf("LocalInputEnd = %d, want 13:40", got)
	}
	if got := LocalInputEnd(views[1:]); got != 0 {
		t.Errorf("only idle, remote and away views: %d, want 0", got)
	}
}

func TestStepSlotsNotOwner(t *testing.T) {
	mk := func(others ...int64) Input {
		in := input(at(14, 0, 30), vw{from: at(13, 0, 0), to: at(13, 45, 0)})
		in.Plan = config.DefaultPlan()
		in.Slots = []Slot{reviewSlot()}
		in.OtherInputMS = others
		return in
	}
	newer := mk(at(14, 0, 20))
	if got := only(Step(newer), SlotDue); len(got) != 0 {
		t.Errorf("another machine has newer input: %+v, want no SlotDue", got)
	}
	if !SlotsDue(newer) {
		t.Error("SlotsDue = false: it must stay ownership-free so the caller loads the other machines")
	}
	if got := only(Step(mk(at(13, 30, 0))), SlotDue); len(got) != 1 {
		t.Errorf("another machine is older: %+v, want the event", got)
	}
	if got := only(Step(mk()), SlotDue); len(got) != 1 {
		t.Errorf("no other machine: %+v, want the event", got)
	}
}
