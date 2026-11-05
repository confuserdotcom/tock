package bar

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"tock/internal/app"
	"tock/internal/config"
	"tock/internal/report"
)

// IDFocus (+ "<minutes>:<goal>") starts a session with a goal; 0 minutes is
// the first preset. IDHotkey asks for the start prompt.
const (
	IDFocus  = "focus:"
	IDHotkey = "hotkey"
)

// Prompt is what the start prompt (the ⌥⌘F window) shows.
type Prompt struct {
	Stop          bool   // a session runs: the prompt offers to stop it
	Title, Detail string // Detail is "" when there is nothing to add
	Presets       []int  // start lengths, the first is the default
}

// Prompt is the start prompt at nowMS.
func (s Snapshot) Prompt(nowMS int64) Prompt {
	if ss := s.Session; ss != nil {
		t := "Stop the session?"
		if ss.Goal != "" {
			t = "Stop " + ss.Goal + "?"
		}
		return Prompt{Stop: true, Title: t, Detail: report.Countdown(ss.PlannedMS-nowMS) + " left"}
	}
	p := s.Presets
	if len(p) == 0 {
		p = []int{25}
	}
	return Prompt{Title: "What are you working on?", Presets: p}
}

// focusID parses "<minutes>:<goal>".
func focusID(v string) (int, string, bool) {
	m, goal, ok := strings.Cut(v, ":")
	n, err := strconv.Atoi(m)
	if !ok || err != nil || n < 0 || n > config.MaxSessionMin {
		return 0, "", false
	}
	return n, goal, true
}

// startGoal runs an IDFocus click.
func startGoal(a *app.App, id string) (int, string, error) {
	n, goal, ok := focusID(strings.TrimPrefix(id, IDFocus))
	if !ok {
		return 0, "", fmt.Errorf("menu %q: %w", id, ErrUnknown)
	}
	if n == 0 {
		n = 25
		if p := a.Config().Focus.Presets; len(p) > 0 {
			n = p[0]
		}
	}
	return n, goal, nil
}

// URLAction is the menu id a tock:// link runs, for Shortcuts and Raycast:
// tock://start?min=50&goal=…, tock://stop, tock://prompt and
// tock://dashboard.
func URLAction(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "tock") {
		return "", false
	}
	switch strings.ToLower(u.Host + u.Opaque) {
	case "start":
		q := u.Query()
		m := 0
		if v := q.Get("min"); v != "" {
			if m, err = strconv.Atoi(v); err != nil || m < 1 || m > config.MaxSessionMin {
				return "", false
			}
		}
		return fmt.Sprintf("%s%d:%s", IDFocus, m, q.Get("goal")), true
	case "stop":
		return IDStop, true
	case "prompt":
		return IDHotkey, true
	case "dashboard":
		return IDDashboard, true
	}
	return "", false
}
