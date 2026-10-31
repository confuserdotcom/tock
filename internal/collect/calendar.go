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
	// Location, URL and Notes are raw text from EventKit (anyone who sends an
	// invite writes them). trackd reads them only to find the meeting link
	// and never stores or logs them.
	Location, URL, Notes string
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

// parseEvents reads the shim's lines: "v2\tstart\tend\tcalendar\tlocation\turl\tnotes\ttitle\n"
// (title last, so it may hold a tab) or the older
// "start\tend\tcalendar\ttitle\n".
func parseEvents(s string) ([]CalendarEvent, error) {
	var out []CalendarEvent
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			continue
		}
		var f []string
		var ev CalendarEvent
		if rest, ok := strings.CutPrefix(line, "v2\t"); ok {
			f = strings.SplitN(rest, "\t", 7)
			if len(f) != 7 {
				return nil, fmt.Errorf("calendar: bad event line")
			}
			ev.Calendar, ev.Location, ev.URL, ev.Notes, ev.Title = f[2], f[3], f[4], f[5], f[6]
		} else {
			f = strings.SplitN(line, "\t", 4)
			if len(f) != 4 {
				return nil, fmt.Errorf("calendar: bad event line")
			}
			ev.Calendar, ev.Title = f[2], f[3]
		}
		start, err1 := strconv.ParseInt(f[0], 10, 64)
		end, err2 := strconv.ParseInt(f[1], 10, 64)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("calendar: bad event times")
		}
		if end > start {
			ev.StartMS, ev.EndMS = start, end
			out = append(out, ev)
		}
	}
	return out, nil
}
