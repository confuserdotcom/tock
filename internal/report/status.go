package report

import (
	"strconv"
	"strings"
)

// StatusInput is what the one-line status needs.
type StatusInput struct {
	TrackedMS, FocusMS int64
	Score              int
	Active             bool // the day has active time, so Score means something
	Open               *Row // the row holding the open span; nil when none is open
	TrackdUp           bool
	InSession          bool  // a manual focus session is running
	SessionLeftMS      int64 // its time left
}

// Status renders the line `tock status` prints, for example
// "5h12 · score 64 · focus 2h40 · now Code / tock".
func Status(in StatusInput) string {
	if !in.TrackdUp {
		return "trackd down · " + Dur(in.TrackedMS)
	}
	parts := []string{Dur(in.TrackedMS)}
	if in.Active {
		parts = append(parts, "score "+strconv.Itoa(in.Score))
	}
	if in.FocusMS > 0 {
		parts = append(parts, "focus "+Dur(in.FocusMS))
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
	if in.InSession {
		parts = append(parts, "◷ "+Dur(max(in.SessionLeftMS, 0)))
	}
	return strings.Join(parts, " · ")
}
