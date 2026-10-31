package focus

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"tock/internal/classify"
)

// Slot kinds: where a slot comes from.
const (
	SlotPlan  = "plan"  // a timed item of the [plan] file
	SlotEvent = "event" // a calendar event
)

// Slot notice kinds, as kept in slot_notice.
const (
	SlotUpcoming = "upcoming" // the up-next notice
	SlotStarting = "start"    // the starting-now notice
)

// Slot notice titles and topics.
const (
	TitleUpcoming = "Up next"
	TitleStarting = "Starting now"
	TopicUpcoming = "upcoming"
	TopicStarting = "starting"
)

const (
	// SlotStartWindowMS is how long after its start a slot may still bring
	// its starting notice; a trackd that first ticks later sends none.
	SlotStartWindowMS = 2 * 60_000
	// SlotFreshMS is the least time a slot has to be known before its start
	// for an up-next notice; closer than that, only the starting one comes.
	SlotFreshMS = 60_000
	// SlotTitleCells is the longest slot title in a notice body, in cells.
	SlotTitleCells = 40
	// SlotCalendarCells is the longest calendar name in a notice body.
	SlotCalendarCells = 24
	// SlotOwnerFreshMS is how far from now another machine's latest input
	// may be, on either side, and still count against this machine owning a
	// slot notice.
	SlotOwnerFreshMS = 15 * 60_000
)

// Slot is a stretch of today that starts at a known time: a timed plan item
// or a calendar event.
type Slot struct {
	// Key identifies the slot across resyncs and restarts; it never holds a
	// row id or a file line number.
	Key  string
	Kind string // SlotPlan or SlotEvent
	// StartMS and EndMS are unix ms.
	StartMS, EndMS int64
	Title          string
	Calendar       string // event: its calendar's name
	Project        string // plan: module of its first resolving +tag
	JoinURL        string // event: allowlisted meeting link, "" = none
	// SeenMS is when the caller first had this slot; 0 = long ago.
	SeenMS int64
}

// SlotSentKey is the SlotSent key of the kind notice of slot key.
func SlotSentKey(key, kind string) string { return key + "|" + kind }

// SlotName is the slot's title as it reads in a notice: words that are web
// links dropped, spaces collapsed, cut to cells; "(no title)" when nothing
// is left.
func SlotName(s Slot, cells int) string {
	var words []string
	for _, w := range strings.Fields(s.Title) {
		if strings.HasPrefix(w, "http://") || strings.HasPrefix(w, "https://") {
			continue
		}
		words = append(words, w)
	}
	if len(words) == 0 {
		return "(no title)"
	}
	return ansi.Truncate(strings.Join(words, " "), cells, "…")
}

// LocalInputEnd is the end of this machine's latest own activity: the latest
// End of a view with a positive ID whose bundle does not start with "tock."
// (idle, away, meeting and call views, and views of other machines carry
// negative ids or tock.* bundles). It is the rule App.exportDay uses for what
// this machine writes to the sync folder. 0 = none.
func LocalInputEnd(views []classify.SpanView) int64 {
	var last int64
	for _, v := range views {
		if v.ID <= 0 || strings.HasPrefix(v.BundleID, "tock.") {
			continue
		}
		last = max(last, v.End)
	}
	return last
}

// SlotOwner reports that this machine sends the slot notices: no other
// synced machine has a latest input end (others, unix ms) later than
// localMS within SlotOwnerFreshMS of nowMS. A machine whose latest input is
// older than that is idle or gone, and one that claims to be further ahead
// of now is wrong about its clock; neither silences this one. A tie
// notifies: a duplicate is better than none.
func SlotOwner(localMS, nowMS int64, others []int64) bool {
	for _, ms := range others {
		if ms > localMS && ms-nowMS <= SlotOwnerFreshMS && nowMS-ms <= SlotOwnerFreshMS {
			return false
		}
	}
	return true
}

// slotLabel is the slot as a notice body names it: its title, and for an
// event with a calendar that calendar's name in parentheses.
func slotLabel(s Slot) string {
	name := SlotName(s, SlotTitleCells)
	if s.Kind == SlotEvent && s.Calendar != "" {
		name += " (" + ansi.Truncate(s.Calendar, SlotCalendarCells, "…") + ")"
	}
	return name
}

// inStartWindow reports Now in [StartMS, StartMS + SlotStartWindowMS).
func inStartWindow(in Input, s Slot) bool {
	return in.Now >= s.StartMS && in.Now < s.StartMS+SlotStartWindowMS
}

// inUpcomingWindow reports Now in [StartMS - NotifyBeforeMS, StartMS) for a
// slot the caller has known for at least SlotFreshMS before its start. A
// slot first seen closer to its start gets no up-next notice, only the
// starting one: "in 0 min" would say nothing the starting notice does not.
// The window never overlaps the starting one.
func inUpcomingWindow(in Input, s Slot) bool {
	return in.Plan.NotifyBeforeMS > 0 && in.Now >= s.StartMS-in.Plan.NotifyBeforeMS && in.Now < s.StartMS &&
		s.StartMS-s.SeenMS >= SlotFreshMS
}

// SlotsDue reports that some slot is in a notice window now (up-next or
// starting), so the caller
// reads which notices it already sent before calling Step.
func SlotsDue(in Input) bool {
	if !in.Plan.Notify {
		return false
	}
	for _, s := range in.Slots {
		if inStartWindow(in, s) || inUpcomingWindow(in, s) {
			return true
		}
	}
	return false
}

// stepSlots emits a SlotDue for each slot in its up-next or starting window
// whose notice was not sent yet, unless another synced machine has the latest
// input (SlotOwner). It never looks at calls or meetings: the point of the
// notice is to tell you.
func stepSlots(in Input) []Event {
	if !in.Plan.Notify || !SlotOwner(LocalInputEnd(in.Views), in.Now, in.OtherInputMS) {
		return nil
	}
	loc := in.Day.Location()
	clock := func(ms int64) string { return time.UnixMilli(ms).In(loc).Format("15:04") }
	var out []Event
	for _, s := range in.Slots {
		if inUpcomingWindow(in, s) && !in.SlotSent[SlotSentKey(s.Key, SlotUpcoming)] {
			n := max((s.StartMS-in.Now+59_999)/60_000, 1)
			out = append(out, Event{Kind: SlotDue, Topic: TopicUpcoming, Title: TitleUpcoming,
				Body: fmt.Sprintf("%s %s · in %d min", clock(s.StartMS), slotLabel(s), n),
				Slot: s, SlotKind: SlotUpcoming})
		}
		if inStartWindow(in, s) && !in.SlotSent[SlotSentKey(s.Key, SlotStarting)] {
			out = append(out, Event{Kind: SlotDue, Topic: TopicStarting, Title: TitleStarting,
				Body: clock(s.StartMS) + "–" + clock(s.EndMS) + " " + slotLabel(s),
				Slot: s, SlotKind: SlotStarting})
		}
	}
	return out
}
