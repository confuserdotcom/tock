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
)

// slotCache is the slot list built from the plan file, with what it was
// built from. It is rebuilt only when one of them changes, so an idle tick
// reads no file.
type slotCache struct {
	file  string
	stamp uint64
	rs    *classify.Ruleset
	day   string
	slots []focus.Slot // SeenMS filled
}

// Slots returns today's slots in start order: the timed, not done items of
// the [plan] file due on the calendar day of now. A slot ends where the next
// later timed item starts, at most PlanSlotMaxMS after its start. A plan
// file that cannot be read gives no slots. The list is kept until the plan
// file, the rules or the day change; a slot's SeenMS is the tick time its key
// first appeared (a restart starts over).
func (a *App) Slots(now time.Time) ([]focus.Slot, error) {
	day := a.midnight(now)
	dayKey := day.Format("2006-01-02")
	c := &a.slotCache
	stamp := a.planStamp()
	if c.slots != nil && c.file == a.cfg.Plan.File && c.stamp == stamp && c.rs == a.rs && c.day == dayKey {
		return slices.Clone(c.slots), nil
	}
	slots := a.planSlotList(day)
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
	*c = slotCache{file: a.cfg.Plan.File, stamp: stamp, rs: a.rs, day: dayKey, slots: slots}
	return slices.Clone(slots), nil
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
		start := day.Add(time.Duration(m) * time.Minute)
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
// its notices were sent. A failure is kept as the tick's RoutineErr (when no
// other is set) and leaves the tick without slot notices.
func (a *App) slotInput(in *focus.Input, nowT time.Time) {
	in.Plan = a.cfg.Plan
	if !in.Plan.Notify {
		return
	}
	skip := func(err error) {
		if a.routineErr == nil {
			a.routineErr = err
		}
		in.Slots, in.SlotSent = nil, nil
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
}
