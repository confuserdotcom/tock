package bar

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/focus"
	"tock/internal/store"
)

// Notification action ids.
const (
	ActStop   = "stop"
	ActAgain  = "again"
	ActSnooze = "snooze"
	ActAway   = "away:" // + index into the notice's labels
	ActIdle   = "idle"
)

// SnoozeFor is how long Snooze holds break reminders back.
const SnoozeFor = 30 * time.Minute

// Button is one notification action.
type Button struct {
	ID, Title string
}

// Buttons are the actions a notice offers.
func Buttons(n store.Notice, d app.NoticeData) []Button {
	switch n.Topic {
	case focus.TopicFocus:
		return []Button{{ActStop, "Stop session"}}
	case focus.TopicDone:
		if d.Minutes > 0 {
			return []Button{{ActAgain, fmt.Sprintf("Start again (%d min)", d.Minutes)}}
		}
	case focus.TopicBreak:
		return []Button{{ActSnooze, fmt.Sprintf("Snooze %d min", int(SnoozeFor.Minutes()))}}
	case focus.TopicAway:
		var out []Button
		for i, w := range d.Labels {
			out = append(out, Button{ActAway + strconv.Itoa(i), WantText(w)})
		}
		return append(out, Button{ActIdle, "Leave idle"})
	}
	return nil
}

// Category is the notification category id of a notice: its topic, or a
// per-notice id for away notices, whose buttons are that notice's labels.
func Category(n store.Notice) string {
	if n.Topic == focus.TopicAway {
		return "away-" + strconv.FormatInt(n.ID, 10)
	}
	return n.Topic
}

// WantText is a label as the TUI writes it: "Category" or "Category /
// project".
func WantText(w classify.Want) string {
	if w.Project == "" {
		return w.Category
	}
	return w.Category + " / " + w.Project
}

// awayIndex parses an away action id.
func awayIndex(action string) (int, bool) {
	v, ok := strings.CutPrefix(action, ActAway)
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(v)
	return i, err == nil && i >= 0
}
