package report

import (
	"strings"
	"time"

	"tock/internal/stats"
)

// OutageWhy says why nothing was recorded in an outage of kind.
func OutageWhy(kind string) string {
	switch kind {
	case stats.GapCrash:
		return "trackd stopped unexpectedly"
	case stats.GapDown:
		return "trackd is not running"
	case stats.GapStopped:
		return "trackd was stopped"
	case stats.GapConfig:
		return "config.toml did not load"
	case stats.GapPermission:
		return "Accessibility permission lost"
	case stats.GapUpdate:
		return "trackd was updating"
	case stats.GapError:
		return "trackd stopped on an error"
	case stats.GapNoAccess:
		return "Accessibility is off: apps only, no window titles"
	}
	return "not recorded"
}

// GapWhy is OutageWhy for g, saying when its end is not known: trackd did
// not run again before the computer restarted, so it ends at the restart
// and the computer may have been off for part of it.
func GapWhy(g stats.Gap) string {
	if g.EndUnknown {
		return OutageWhy(g.Kind) + " (until the restart; the computer may have been off for part of it)"
	}
	return OutageWhy(g.Kind)
}

// OutagesLine lists outages as "12:30–13:10 trackd stopped unexpectedly ·
// 13:40–14:10 config.toml did not load", in loc; "" without any.
func OutagesLine(gs []stats.Gap, loc *time.Location) string {
	parts := make([]string, len(gs))
	for i, g := range gs {
		parts[i] = time.UnixMilli(g.From).In(loc).Format("15:04") + "–" +
			time.UnixMilli(g.To).In(loc).Format("15:04") + " " + GapWhy(g)
	}
	return strings.Join(parts, " · ")
}

// SplitPartial splits gs into the time nothing was recorded and the
// apps-only time, each in order.
func SplitPartial(gs []stats.Gap) (none, partial []stats.Gap) {
	for _, g := range gs {
		if g.Partial() {
			partial = append(partial, g)
		} else {
			none = append(none, g)
		}
	}
	return none, partial
}
