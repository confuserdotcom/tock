//go:build !(darwin && cgo)

package collect

import "errors"

// EventKit is unavailable without darwin and cgo: it reports off.
type EventKit struct{}

// Status is always off.
func (EventKit) Status() string { return CalendarOff }

// Request is always off.
func (EventKit) Request() string { return CalendarOff }

// Events always fails.
func (EventKit) Events(from, to int64, calendars []string) ([]CalendarEvent, error) {
	return nil, errors.New("calendar: needs macOS and cgo")
}
