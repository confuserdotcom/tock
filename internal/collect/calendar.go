package collect

import (
	"fmt"
	"strconv"
	"strings"
)

// Calendar access states, as trackd records them in daemon_state.
const (
	CalendarOff          = "off"
	CalendarGranted      = "granted"
	CalendarDenied       = "denied"
	CalendarUndetermined = "undetermined"
	CalendarError        = "error"
)

// CalendarEvent is one timed event: all-day, free and declined events are
// never returned.
type CalendarEvent struct {
	StartMS, EndMS  int64
	Title, Calendar string
}

// calendarStatus maps EKAuthorizationStatus to an access state: full access
// is granted, write-only access is as good as denied.
func calendarStatus(s int) string {
	switch s {
	case 0:
		return CalendarUndetermined
	case 3:
		return CalendarGranted
	case 1, 2, 4:
		return CalendarDenied
	}
	return CalendarError
}

// parseEvents reads the shim's "start\tend\tcalendar\ttitle\n" lines.
func parseEvents(s string) ([]CalendarEvent, error) {
	var out []CalendarEvent
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, "\t", 4)
		if len(f) != 4 {
			return nil, fmt.Errorf("calendar: bad event line")
		}
		start, err1 := strconv.ParseInt(f[0], 10, 64)
		end, err2 := strconv.ParseInt(f[1], 10, 64)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("calendar: bad event times")
		}
		if end > start {
			out = append(out, CalendarEvent{StartMS: start, EndMS: end, Calendar: f[2], Title: f[3]})
		}
	}
	return out, nil
}
