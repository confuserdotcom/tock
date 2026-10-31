package focus

import (
	"strings"
	"testing"

	"tock/internal/config"
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
