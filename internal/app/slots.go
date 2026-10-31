package app

import (
	"fmt"
	"hash/fnv"
	"slices"
	"sort"
	"time"

	"tock/internal/classify"
	"tock/internal/focus"
	"tock/internal/plan"
	"tock/internal/store"
)

// slotRefresh is how often Slots looks at the plan file and the event table
// to see whether they changed: not on every 5 s tick, so an idle tick stats no
// file and runs no query. A notice window is at least 2 min, so an edit seen
// up to a minute late still notifies on time.
const slotRefresh = time.Minute

// slotCache is the slot list built from the plan file, with what it was
// built from. It is rebuilt only when one of them changes, so an idle tick
// reads no file.
type slotCache struct {
	// checkedMS is the tick time (ms) the plan file and the event table were
	// last looked at.
	checkedMS int64
	file      string
	stamp     uint64
	rs        *classify.Ruleset
	day       string
	// events is the (count, max id) of the event table the slots were built
	// from.
	eventN, eventMax int64
	slots            []focus.Slot // SeenMS filled
}

// Slots returns today's slots in start order (then key): the timed, not done
// items of the [plan] file due on the calendar day of now, and the calendar
// events overlapping the day. A plan slot ends where the next later timed
// item starts, at most PlanSlotMaxMS after its start. A plan file that
// cannot be read gives no plan slots; an event table that cannot be read is
// an error. The list is kept until the plan file, the rules, the event table
// or the day change (the plan file and the event table are looked at once per
// slotRefresh); a slot's SeenMS is the tick time its key first appeared
// (a restart starts over).
func (a *App) Slots(now time.Time) ([]focus.Slot, error) {
	day := a.midnight(now)
	dayKey := day.Format("2006-01-02")
	c := &a.slotCache
	nowMS := now.UnixMilli()
	same := c.slots != nil && c.file == a.cfg.Plan.File && c.rs == a.rs && c.day == dayKey
	if age := nowMS - c.checkedMS; same && age >= 0 && age < slotRefresh.Milliseconds() {
		return slices.Clone(c.slots), nil
	}
	stamp := a.planStamp()
	n, maxID, err := a.st.EventStamp()
	if err != nil {
		return nil, fmt.Errorf("event stamp: %w", err)
	}
	if same && c.stamp == stamp && c.eventN == n && c.eventMax == maxID {
		c.checkedMS = nowMS
		return slices.Clone(c.slots), nil
	}
	evs, err := a.st.Events(day.UnixMilli(), day.AddDate(0, 0, 1).UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("events: %w", err)
	}
	slots := a.planSlotList(day)
	haveEvent := map[string]bool{}
	for _, ev := range evs {
		key := eventSlotKey(ev)
		if haveEvent[key] { // the same event in two calendars notifies once
			continue
		}
		haveEvent[key] = true
		slots = append(slots, focus.Slot{Key: key, Kind: focus.SlotEvent, StartMS: ev.StartMS,
			EndMS: ev.EndMS, Title: ev.Title, Calendar: ev.Calendar, JoinURL: ev.JoinURL, Mine: ev.Mine})
	}
	sort.SliceStable(slots, func(i, j int) bool {
		if slots[i].StartMS != slots[j].StartMS {
			return slots[i].StartMS < slots[j].StartMS
		}
		return slots[i].Key < slots[j].Key
	})
	seen := make(map[string]int64, len(slots))
	for i := range slots {
		ms, ok := a.slotSeen[slots[i].Key]
		if !ok {
			ms = now.UnixMilli()
		}
		seen[slots[i].Key] = ms
		slots[i].SeenMS = ms
	}
	a.slotSeen = seen
	if slots == nil {
		slots = []focus.Slot{} // an empty list is still a built one
	}
	*c = slotCache{checkedMS: nowMS, file: a.cfg.Plan.File, stamp: stamp, rs: a.rs, day: dayKey, eventN: n, eventMax: maxID, slots: slots}
	return slices.Clone(slots), nil
}

// eventSlotKey is the key of an event slot: its start and a hash of its
// title. Row ids change when trackd syncs the calendar again, so they never
// take part.
func eventSlotKey(ev store.Event) string {
	h := fnv.New64a()
	h.Write([]byte(ev.Title))
	return fmt.Sprintf("event:%d:%016x", ev.StartMS, h.Sum64())
}

// planSlotList reads the plan file and builds the slots of day.
func (a *App) planSlotList(day time.Time) []focus.Slot {
	if a.cfg.Plan.File == "" {
		return nil
	}
	lines, err := plan.Read(a.cfg.Plan.File)
	if err != nil {
		return nil
	}
	due := day.Format("2006-01-02")
	var out []focus.Slot
	have := map[string]bool{}
	for _, l := range lines {
		it := plan.Parse(l)
		m, ok := plan.Clock(it)
		if !ok || it.Done || it.Due != due {
			continue
		}
		// Wall-clock time, not midnight plus elapsed time: a zone change
		// (DST) between midnight and the item must not shift it.
		y, mo, d := day.Date()
		start := time.Date(y, mo, d, m/60, m%60, 0, 0, day.Location())
		h := fnv.New64a()
		h.Write([]byte(it.Text))
		key := fmt.Sprintf("plan:%sT%s:%016x", due, start.Format("15:04"), h.Sum64())
		if have[key] {
			continue
		}
		have[key] = true
		project := ""
		for _, p := range it.Projects {
			if project = a.rs.ModuleOf(p); project != "" {
				break
			}
		}
		out = append(out, focus.Slot{Key: key, Kind: focus.SlotPlan, StartMS: start.UnixMilli(),
			EndMS: start.UnixMilli() + PlanSlotMaxMS, Title: it.Text, Project: project})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StartMS != out[j].StartMS {
			return out[i].StartMS < out[j].StartMS
		}
		return out[i].Key < out[j].Key
	})
	for i := range out {
		for _, later := range out[i+1:] {
			if later.StartMS > out[i].StartMS {
				out[i].EndMS = min(out[i].EndMS, later.StartMS)
				break
			}
		}
	}
	return out
}

// slotInput fills in the slot part of a tick's input: the [plan] switches
// and today's slots, and, only when a slot is in a notice window, which of
// its notices were sent and the other synced machines' latest input (who
// owns the notices). A failure is kept as the tick's RoutineErr (when no
// other is set) and leaves the tick without slot notices.
func (a *App) slotInput(in *focus.Input, nowT time.Time) {
	in.Plan = a.cfg.Plan
	if !a.cfg.Focus.Notify {
		// No notice reaches anyone, so none is marked as sent; auto_start
		// still starts its session (silently).
		in.Plan.Notify = false
	}
	if !in.Plan.Notify && !in.Plan.AutoStart {
		return
	}
	skip := func(err error) {
		if a.routineErr == nil {
			a.routineErr = err
		}
		in.Slots, in.SlotSent, in.OtherInputMS = nil, nil, nil
	}
	slots, err := a.Slots(nowT)
	if err != nil {
		skip(fmt.Errorf("app: slots: %w", err))
		return
	}
	in.Slots = slots
	if !focus.SlotsDue(*in) {
		return
	}
	keys := make([]string, len(slots))
	for i, s := range slots {
		keys[i] = s.Key
	}
	rows, err := a.st.SlotNotices(keys)
	if err != nil {
		skip(fmt.Errorf("app: slot notices: %w", err))
		return
	}
	in.SlotSent = make(map[string]bool, len(rows))
	for _, r := range rows {
		in.SlotSent[focus.SlotSentKey(r.Key, r.Kind)] = true
	}
	machines, err := a.st.RemoteMachines()
	if err != nil {
		skip(fmt.Errorf("app: slot owner: %w", err))
		return
	}
	for _, m := range machines {
		if m.LastMS > 0 {
			in.OtherInputMS = append(in.OtherInputMS, m.LastMS)
		}
	}
}
