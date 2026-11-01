// Package recommend learns, from history alone, when the user focuses best and
// for how long, and what they answered to earlier suggestions. It is pure: no
// I/O, no clock; callers pass NowMS and every history slice.
package recommend

// Kinds of recommendation and the two replies.
const (
	KindFocus    = "focus"
	KindBreak    = "break"
	ReplyAccept  = "accept"
	ReplyDismiss = "dismiss"
)

// Windows and thresholds of the learning rules.
const (
	DayMS       = 24 * 3600_000
	HourDays    = 28 // days of history behind the hour scores
	SessionDays = 60 // days of finished sessions behind the typical length
	ReplyDays   = 30 // days of replies that count
	MinSamples  = 3  // days of data a (weekday, hour) needs to rank
	TopHours    = 3  // ranked hours per weekday
	// MinHourMS is the mean focus time an hour needs to be a top hour.
	MinHourMS = 15 * 60_000
	// PendingMS is how long a recommendation stays open for a reply; an
	// unanswered one after that is ignored.
	PendingMS = 30 * 60_000
)

// Rec is one recommendation notice that was sent. It has the same fields in
// the same order as store.Recommend; keep them in step.
type Rec struct {
	ID       int64
	Kind     string
	AtMS     int64
	Weekday  int
	Hour     int
	LengthMS int64
	Reply    string
	ReplyMS  int64
}

// Open reports whether the recommendation can still be answered at nowMS:
// unanswered and sent less than PendingMS ago.
func (r Rec) Open(nowMS int64) bool {
	return r.Reply == "" && nowMS >= r.AtMS && nowMS-r.AtMS < PendingMS
}

// Pending returns the newest recommendation still open at nowMS, nil if none.
func Pending(recs []Rec, nowMS int64) *Rec {
	var best *Rec
	for i := range recs {
		r := &recs[i]
		if r.Open(nowMS) && (best == nil || r.AtMS > best.AtMS || r.AtMS == best.AtMS && r.ID > best.ID) {
			best = r
		}
	}
	return best
}

// CountSince counts the recommendations of kind sent at or after fromMS.
func CountSince(recs []Rec, kind string, fromMS int64) int {
	n := 0
	for _, r := range recs {
		if r.Kind == kind && r.AtMS >= fromMS {
			n++
		}
	}
	return n
}

// Last returns the latest AtMS among the recommendations of kind, 0 when none.
func Last(recs []Rec, kind string) int64 {
	var at int64
	for _, r := range recs {
		if r.Kind == kind && r.AtMS > at {
			at = r.AtMS
		}
	}
	return at
}
