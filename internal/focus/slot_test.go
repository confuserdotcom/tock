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

// moduleSlot is the TD 1 slot of module Analyse 1, 14:00-14:50.
func moduleSlot() Slot {
	return Slot{Key: "plan:2026-10-10T14:00:t", Kind: SlotPlan, StartMS: at(14, 0, 0), EndMS: at(14, 50, 0), Title: "TD 1", Project: "Analyse 1"}
}

// autoInput is a tick at now with auto_start on and the module slot.
func autoInput(now int64) Input {
	in := slotInput(now)
	in.Plan.AutoStart = true
	in.Slots = []Slot{moduleSlot()}
	return in
}

func onlyStart(t *testing.T, in Input) Event {
	t.Helper()
	evs := only(Step(in), SlotDue)
	if len(evs) != 1 || evs[0].SlotKind != SlotStarting {
		t.Fatalf("events = %+v, want exactly the starting SlotDue", evs)
	}
	return evs[0]
}

func runningManual() []Session {
	return []Session{{ID: 3, Kind: KindManual, StartMS: at(13, 58, 0), PlannedMS: at(14, 23, 0)}}
}

func TestStepSlotAutoStart(t *testing.T) {
	e := onlyStart(t, autoInput(at(14, 0, 30)))
	wantSess := Session{Kind: KindManual, StartMS: at(14, 0, 30), PlannedMS: at(14, 50, 0)}
	if !e.AutoStart || e.Sess != wantSess || e.Silent || e.Minutes != 0 ||
		e.Body != "14:00–14:50 TD 1 · focus session started · 50 min · Analyse 1" || e.AltBody != "14:00–14:50 TD 1" {
		t.Fatalf("event = %+v, want auto start of %+v", e, wantSess)
	}

	short := autoInput(at(14, 0, 30))
	short.Slots[0].EndMS = at(14, 2, 0)
	e = onlyStart(t, short)
	if e.Sess.PlannedMS != at(14, 0, 30)+SlotSessionMinMS || !strings.Contains(e.Body, "· 5 min · Analyse 1") {
		t.Errorf("short slot: planned %d, body %q, want 5 min", e.Sess.PlannedMS, e.Body)
	}

	long := autoInput(at(14, 0, 30))
	long.Slots[0].EndMS = at(14, 0, 0) + 9*3_600_000
	e = onlyStart(t, long)
	if want := at(14, 0, 30) + int64(config.MaxSessionMin)*60_000; e.Sess.PlannedMS != want {
		t.Errorf("long slot: planned %d, want %d (MaxSessionMin)", e.Sess.PlannedMS, want)
	}

	cut := autoInput(at(14, 0, 30))
	cut.Slots[0].Project = strings.Repeat("p", 30)
	if e = onlyStart(t, cut); !strings.HasSuffix(e.Body, "· "+strings.Repeat("p", 23)+"…") {
		t.Errorf("long project: body %q, want it cut to 24 cells", e.Body)
	}

	meeting := autoInput(at(14, 0, 30))
	meeting.Cfg.MeetingApps = []string{"us.zoom.xos"}
	meeting.Views, meeting.Labels = world(vw{from: at(13, 50, 0), to: at(14, 0, 30), bundle: "us.zoom.xos"})
	for name, in := range map[string]Input{
		"running session": func() Input { in := autoInput(at(14, 0, 30)); in.Sessions = runningManual(); return in }(),
		"call":            func() Input { in := autoInput(at(14, 0, 30)); in.Call = true; return in }(),
		"meeting view":    meeting,
		"no module":       func() Input { in := autoInput(at(14, 0, 30)); in.Slots[0].Project = ""; return in }(),
		"event":           func() Input { in := autoInput(at(14, 0, 30)); in.Slots[0].Kind = SlotEvent; return in }(),
		"off":             func() Input { in := autoInput(at(14, 0, 30)); in.Plan.AutoStart = false; return in }(),
	} {
		e := onlyStart(t, in)
		if e.AutoStart || e.Sess != (Session{}) || e.Silent || e.Body != "14:00–14:50 TD 1" {
			t.Errorf("%s: event = %+v, want the plain body and no start", name, e)
		}
		if e.Minutes != 0 && name != "off" {
			t.Errorf("%s: Minutes = %d, want 0", name, e.Minutes)
		}
	}
}

func TestStepSlotAutoStartNotifyOff(t *testing.T) {
	in := autoInput(at(13, 56, 0))
	in.Plan.Notify = false
	if got := only(Step(in), SlotDue); len(got) != 0 || SlotsDue(in) {
		t.Errorf("13:56: events %+v, SlotsDue %v, want none (no up-next)", got, SlotsDue(in))
	}
	in = autoInput(at(14, 0, 30))
	in.Plan.Notify = false
	if !SlotsDue(in) {
		t.Fatal("SlotsDue = false in the starting window with auto_start on")
	}
	e := onlyStart(t, in)
	if !e.AutoStart || !e.Silent {
		t.Errorf("event = %+v, want AutoStart and Silent", e)
	}
	// A slot that would not start a session sends nothing at all.
	in.Slots[0].Project = ""
	if got := only(Step(in), SlotDue); len(got) != 0 {
		t.Errorf("no module: %+v, want none", got)
	}
	// A running session: the start is not made, so nothing is sent.
	in = autoInput(at(14, 0, 30))
	in.Plan.Notify = false
	in.Sessions = runningManual()
	if got := only(Step(in), SlotDue); len(got) != 0 {
		t.Errorf("running session: %+v, want none", got)
	}
	in = autoInput(at(14, 0, 30))
	in.Plan.Notify, in.Plan.AutoStart = false, false
	if got := only(Step(in), SlotDue); len(got) != 0 || SlotsDue(in) {
		t.Errorf("both off: events %+v, SlotsDue %v", got, SlotsDue(in))
	}
}

func TestStepSlotStartFocusMinutes(t *testing.T) {
	mk := func(end int64, project string) Input {
		in := slotInput(at(14, 0, 30))
		in.Slots = []Slot{moduleSlot()}
		in.Slots[0].EndMS, in.Slots[0].Project = end, project
		return in
	}
	for _, tt := range []struct {
		name string
		in   Input
		want int
	}{
		{"50 min", mk(at(14, 50, 0), "Analyse 1"), 50},
		{"3 min raised to 5", mk(at(14, 3, 0), "Analyse 1"), 5},
		{"9 h cut to the maximum", mk(at(14, 0, 0)+9*3_600_000, "Analyse 1"), config.MaxSessionMin},
		{"no module", mk(at(14, 50, 0), ""), 0},
		{"running session", func() Input {
			in := mk(at(14, 50, 0), "Analyse 1")
			in.Sessions = runningManual()
			return in
		}(), 0},
	} {
		e := onlyStart(t, tt.in)
		if e.Minutes != tt.want || e.AutoStart || e.Silent {
			t.Errorf("%s: Minutes = %d AutoStart %v Silent %v, want %d", tt.name, e.Minutes, e.AutoStart, e.Silent, tt.want)
		}
	}
	// Events never offer it.
	in := mk(at(14, 50, 0), "Analyse 1")
	in.Slots[0].Kind = SlotEvent
	if e := onlyStart(t, in); e.Minutes != 0 {
		t.Errorf("event: Minutes = %d, want 0", e.Minutes)
	}
	// The up-next notice never does.
	in = mk(at(14, 50, 0), "Analyse 1")
	in.Now = at(13, 56, 0)
	if evs := only(Step(in), SlotDue); len(evs) != 1 || evs[0].Minutes != 0 || evs[0].AutoStart {
		t.Errorf("up-next: %+v, want no minutes and no start", evs)
	}
}
