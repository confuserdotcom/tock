// Package span holds the pure sample-to-span logic and the shared type
// contract between collect, store and cmd/trackd. It performs no I/O, reads
// no clock (D-15) and imports no cgo package, so it builds with CGO_ENABLED=0.
//
// All timestamps are int64 unix milliseconds UTC taken from the wall clock
// (never the monotonic reading: Go's monotonic clock does not advance during
// sleep on darwin, so use time.Now().Round(0).UnixMilli()).
package span

import "time"

const (
	// IdleBundleID is the reserved bundle id of idle spans. Phase 2 rules must
	// treat this sentinel specially.
	IdleBundleID = "tock.idle"
	// IdleApp is the app name stored on idle spans.
	IdleApp = "Idle"
	// LoginWindowBundleID is the frontmost bundle while the console is locked.
	LoginWindowBundleID = "com.apple.loginwindow"
	// CallBundleID is the bundle id of call time that trackd recorded as
	// idle (a call you only listen to). It never appears in the span table:
	// views are rewritten at read time.
	CallBundleID = "tock.call"
	// CallApp is the app name of CallBundleID views.
	CallApp = "Call"
	// AwayBundleID is the bundle id of time away that the user labelled
	// (tock away): idle time and gaps rewritten at read time, never stored
	// in the span table.
	AwayBundleID = "tock.away"
	// AwayApp is the app name of AwayBundleID views.
	AwayApp = "Away"
	// MeetingBundleID is the bundle id of time away that a calendar event
	// covers: idle time and gaps rewritten at read time, never stored in
	// the span table.
	MeetingBundleID = "tock.meeting"
	// MeetingApp is the app name of MeetingBundleID views.
	MeetingApp = "Meeting"
	// EntryBundleID marks time the user added by hand (tock edit add).
	EntryBundleID = "tock.entry"
	// EntryApp is the app name of an added entry.
	EntryApp = "Entry"
)

// Automation status values carried by Sample.Automation.
const (
	AutomationGranted    = "granted"
	AutomationDenied     = "denied"
	AutomationUnknown    = "unknown"
	AutomationNotRunning = "not_running"
)

// EventKind identifies a hard boundary event. The C shim uses the same
// numeric codes 1..4.
type EventKind int

const (
	EventSleep  EventKind = 1
	EventWake   EventKind = 2
	EventLock   EventKind = 3
	EventUnlock EventKind = 4
)

// String returns sleep|wake|lock|unlock.
func (k EventKind) String() string {
	switch k {
	case EventSleep:
		return "sleep"
	case EventWake:
		return "wake"
	case EventLock:
		return "lock"
	case EventUnlock:
		return "unlock"
	}
	return "unknown"
}

// Sample is one observation of the frontmost window.
//
// Title nil means unknown (no Accessibility, AX error, or private window).
// URL nil means none; when set it is already reduced to host+path.
// Automation is "" when the frontmost app is not a supported browser.
type Sample struct {
	TS         int64
	BundleID   string
	App        string
	Title      *string
	URL        *string
	Private    bool
	IdleS      float64
	Locked     bool
	Activation bool
	AXTrusted  bool
	Automation string
	// Video: the front app (or a helper of it) keeps the display awake, as
	// players do while a video plays. The merger then waits VideoIdleMax
	// without input before it goes idle.
	Video bool
}

// Event is a sleep/wake/lock/unlock marker.
type Event struct {
	TS   int64
	Kind EventKind
}

// Input is what the collector sends to the merger. Exactly one field is
// non-nil.
type Input struct {
	Sample *Sample
	Event  *Event
}

// Span is one row of the span table.
type Span struct {
	ID       int64
	Start    int64
	End      *int64
	BundleID string
	App      string
	Title    *string
	URL      *string
	Private  bool
}

// OpKind is the kind of a storage operation emitted by the merger.
type OpKind int

const (
	OpOpen      OpKind = 1
	OpClose     OpKind = 2
	OpDelete    OpKind = 3
	OpExtend    OpKind = 4
	OpReopen    OpKind = 5
	OpHeartbeat OpKind = 6
)

// String returns a lowercase name for the op kind.
func (k OpKind) String() string {
	switch k {
	case OpOpen:
		return "open"
	case OpClose:
		return "close"
	case OpDelete:
		return "delete"
	case OpExtend:
		return "extend"
	case OpReopen:
		return "reopen"
	case OpHeartbeat:
		return "heartbeat"
	}
	return "unknown"
}

// Op is a storage operation. Field use per kind:
//
//	Open      uses Span (End nil)
//	Close     uses ID and TS as the new end
//	Delete    uses ID
//	Extend    uses ID and TS as the new end of an already-closed span
//	Reopen    uses ID and sets end back to NULL
//	Heartbeat uses TS
type Op struct {
	Kind OpKind
	Span Span
	ID   int64
	TS   int64
}

// Config tunes the merger. Rule semantics (implemented across plans 01-01..03):
//
//   - idle backdating (D-11): idle at or above IdleThreshold ends the current
//     span at the last activity time and opens an idle span. While a video
//     plays (Sample.Video) the threshold is VideoIdleMax.
//   - hard boundaries (D-12, refined): sleep, wake, lock and unlock close the
//     open span at the last known-good time.
//   - fold (D-13): non-idle spans shorter than FoldMin are folded into a
//     neighbour.
//   - clock jump (D-14): a gap larger than JumpMax in either direction closes
//     the open span at the last known-good time.
//   - heartbeat (D-16): a heartbeat op is emitted once Heartbeat-HeartbeatLead
//     has passed since the previous one, so crash loss stays within Heartbeat
//     even when polls arrive up to 500 ms late. HeartbeatLead (2500 ms) is one
//     2 s poll interval plus 500 ms poll lateness.
//   - TitleKey nil means identity.
//   - a private sample's title and URL are never stored (D-07).
type Config struct {
	IdleThreshold time.Duration
	VideoIdleMax  time.Duration // idle threshold while Sample.Video; 0 = IdleThreshold
	FoldMin       time.Duration
	JumpMax       time.Duration
	Heartbeat     time.Duration
	HeartbeatLead time.Duration
	TitleKey      func(string) string
	NextID        int64
}

// DefaultConfig returns the Phase 1 constants.
func DefaultConfig() Config {
	return Config{
		IdleThreshold: 5 * time.Minute,
		VideoIdleMax:  3 * time.Hour,
		FoldMin:       2 * time.Second,
		JumpMax:       2 * time.Minute,
		Heartbeat:     10 * time.Second,
		HeartbeatLead: 2500 * time.Millisecond,
		TitleKey:      nil,
		NextID:        1,
	}
}
