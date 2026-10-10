package report

import "strings"

// StatusInput is what the one-line status needs.
type StatusInput struct {
	TrackedMS, DeepMS int64
	Open              *Row // the row holding the open span; nil when none is open
	TrackdUp          bool
}

// Status renders the line `tock status` prints, for example
// "5h12 · deep 2h40 · now Code / tock".
func Status(in StatusInput) string {
	if !in.TrackdUp {
		return "trackd down · " + Dur(in.TrackedMS)
	}
	parts := []string{Dur(in.TrackedMS)}
	if in.DeepMS > 0 {
		parts = append(parts, "deep "+Dur(in.DeepMS))
	}
	if r := in.Open; r != nil {
		switch {
		case r.Idle:
			parts = append(parts, "now idle")
		case r.Label.Category == "" && r.Label.Project == "":
			parts = append(parts, "now unlabelled")
		default:
			parts = append(parts, "now "+LabelText(r.Label.Category, r.Label.Project))
		}
	}
	return strings.Join(parts, " · ")
}
