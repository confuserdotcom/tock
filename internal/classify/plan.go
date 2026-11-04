package classify

// PlanSlot is a planned task's time with the module its +tag names.
type PlanSlot struct {
	Interval
	Project string
}
