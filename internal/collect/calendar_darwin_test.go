//go:build darwin && cgo

package collect

import "testing"

// TestEventKitStatus reads the status only: it never prompts.
func TestEventKitStatus(t *testing.T) {
	switch s := (EventKit{}).Status(); s {
	case CalendarGranted, CalendarDenied, CalendarUndetermined:
	default:
		t.Fatalf("Status() = %q", s)
	}
}
