package classify

// PlanSlot is a planned task's time with the module its +tag names.
type PlanSlot struct {
	Interval
	Project string
}

// MarkPlan splits views at slot edges and sets Planned on the pieces
// inside a slot. Every piece keeps its span's ID. slots must be sorted by
// From and must not overlap.
func MarkPlan(views []SpanView, slots []PlanSlot) []SpanView {
	if len(slots) == 0 {
		return views
	}
	out := make([]SpanView, 0, len(views)+2*len(slots))
	for _, v := range views {
		at := v.Start
		for _, s := range slots {
			if s.To <= at || s.From >= v.End {
				continue
			}
			if s.From > at {
				p := v
				p.Start, p.End = at, s.From
				out = append(out, p)
				at = s.From
			}
			p := v
			p.Start, p.End, p.Planned = at, min(s.To, v.End), s.Project
			out = append(out, p)
			at = p.End
		}
		if at < v.End {
			p := v
			p.Start = at
			out = append(out, p)
		}
	}
	return out
}
