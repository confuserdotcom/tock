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
	}
	return "not recorded"
}

// OutagesLine lists outages as "12:30–13:10 trackd stopped unexpectedly ·
// 13:40–14:10 config.toml did not load", in loc; "" without any.
func OutagesLine(gs []stats.Gap, loc *time.Location) string {
	parts := make([]string, len(gs))
	for i, g := range gs {
		parts[i] = time.UnixMilli(g.From).In(loc).Format("15:04") + "–" +
			time.UnixMilli(g.To).In(loc).Format("15:04") + " " + OutageWhy(g.Kind)
	}
	return strings.Join(parts, " · ")
}
