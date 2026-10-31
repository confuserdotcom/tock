package bar

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/store"
)

// Notification action ids.
const (
	ActStop   = "stop"
	ActAgain  = "again"
	ActSnooze = "snooze"
	ActAway   = "away:"   // + index into the notice's labels
	ActRate   = "rate:"   // + rating 1..5, on a done notice
	ActRefile = "refile:" // refile the finished session to the notice's top project
	ActIdle   = "idle"
	// ActSnoozeDistract snoozes distraction notices for the notice's minutes.
	ActSnoozeDistract = "snooze:distract"
)

// ProjectCells is how wide a project name may be in a button or menu item.
const ProjectCells = 24

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
		var out []Button
		if d.SnoozeMin > 0 {
			out = append(out, Button{ActSnoozeDistract, fmt.Sprintf("Snooze %d min", d.SnoozeMin)})
		}
		return append(out, Button{ActStop, "Stop session"})
	case focus.TopicDone:
		// The first action is the one macOS shows as the button.
		var out []Button
		if d.Session > 0 {
			for r := 5; r >= 1; r-- {
				out = append(out, Button{ActRate + strconv.Itoa(r), fmt.Sprintf("Rate ★%d", r)})
			}
			if d.Project != "" && d.Refile {
				out = append(out, Button{ActRefile, "Refile → " + report.Cut(d.Project, ProjectCells)})
			}
		}
		if d.Minutes > 0 {
			out = append(out, Button{ActAgain, fmt.Sprintf("Start again (%d min)", d.Minutes)})
		}
		return out
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

// Category is the notification category id of a notice with buttons: one
// per notice, since the buttons depend on its data. "" for none.
func Category(n store.Notice, d app.NoticeData) string {
	if len(Buttons(n, d)) == 0 {
		return ""
	}
	return n.Topic + "-" + strconv.FormatInt(n.ID, 10)
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
