//go:build darwin && cgo

package collect

/*
#cgo LDFLAGS: -framework EventKit -framework Foundation
#include <stdlib.h>
#include "calendar_darwin.h"
*/
import "C"

import (
	"errors"
	"strings"
	"unsafe"
)

// EventKit reads calendar events through macOS Calendar. It needs the
// calendar permission, which only a launchd-started trackd should request.
type EventKit struct{}

// Status is the current access state.
func (EventKit) Status() string { return calendarStatus(int(C.tock_calendar_status())) }

// Request asks for access when it is undetermined (macOS shows a prompt) and
// returns the new state. It blocks until the user answers, at most two
// minutes.
func (EventKit) Request() string { return calendarStatus(int(C.tock_calendar_request())) }

// Events returns the timed events overlapping [from, to) in the named
// calendars (all when empty).
func (EventKit) Events(from, to int64, calendars []string) ([]CalendarEvent, error) {
	names := C.CString(strings.Join(calendars, "\n"))
	defer C.free(unsafe.Pointer(names))
	out := C.tock_calendar_events(C.int64_t(from), C.int64_t(to), names)
	if out == nil {
		return nil, errors.New("calendar: EventKit read failed")
	}
	defer C.free(unsafe.Pointer(out))
	return parseEvents(C.GoString(out))
}
