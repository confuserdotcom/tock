package app

import (
	"os"
	"sort"
	"time"

	"tock/internal/classify"
	"tock/internal/plan"
)

// PlanSlotMaxMS is the longest a planned task's slot runs when no later
// task that day ends it.
const PlanSlotMaxMS = 3 * 3_600_000

// planSlots reads the [plan] file and returns the slots of its timed tasks
// due in [from, to) whose +tag names a module: from the task's time to the
// next timed task that day, at most PlanSlotMaxMS.
func (a *App) planSlots(from, to int64) []classify.PlanSlot {
	if a.cfg.Plan.File == "" {
		return nil
	}
	lines, err := plan.Read(a.cfg.Plan.File)
	if err != nil || len(lines) == 0 {
		return nil
	}
	type timed struct {
		at      int64
		project string
	}
	byDay := map[string][]timed{}
	loc := a.now().Location()
	for _, l := range lines {
		it := plan.Parse(l)
		m, ok := plan.Clock(it)
		if !ok || it.Due == "" {
			continue
		}
		day, err := time.ParseInLocation("2006-01-02", it.Due, loc)
		if err != nil {
			continue
		}
		at := day.Add(time.Duration(m) * time.Minute).UnixMilli()
		project := ""
		for _, p := range it.Projects {
			if project = a.rs.ModuleOf(p); project != "" {
				break
			}
		}
		byDay[it.Due] = append(byDay[it.Due], timed{at, project})
	}
	var out []classify.PlanSlot
	for _, ts := range byDay {
		sort.SliceStable(ts, func(i, j int) bool { return ts[i].at < ts[j].at })
		for i, t := range ts {
			if t.project == "" {
				continue
			}
			end := t.at + PlanSlotMaxMS
			if i+1 < len(ts) && ts[i+1].at > t.at {
				end = min(end, ts[i+1].at)
			}
			if end > from && t.at < to && end > t.at {
				out = append(out, classify.PlanSlot{Interval: classify.Interval{From: t.at, To: end}, Project: t.project})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	// Two tasks at the same time: the first keeps the slot.
	kept := out[:0]
	for _, s := range out {
		if len(kept) > 0 && s.From < kept[len(kept)-1].To {
			continue
		}
		kept = append(kept, s)
	}
	return kept
}

// planStamp changes when the [plan] file does.
func (a *App) planStamp() uint64 {
	if a.cfg.Plan.File == "" {
		return 0
	}
	fi, err := os.Stat(a.cfg.Plan.File)
	if err != nil {
		return 0
	}
	return uint64(fi.ModTime().UnixNano()) ^ uint64(fi.Size())<<20
}
