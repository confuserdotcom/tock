package focus

import (
	"slices"
	"strings"

	"tock/internal/classify"
)

// GuardState is the focus guard's memory between ticks, kept in
// daemon_state.
type GuardState struct {
	HookSession int64 // id of the session whose start hooks ran; 0 = none
	BlockAt     int64 // when the last block happened
}

// Block asks trackd to take a distraction out of the way: close the front
// tab when Host is set and still in front, else hide the app.
type Block struct {
	Bundle, App string
	Host        string // the URL host; "" when the view has no URL
	LeftMS      int64  // time left in the session
}

// Guard is what one tick asks trackd to do around the running session.
type Guard struct {
	Start   *Session // a session began: run the start hooks
	Stop    bool     // the session the hooks ran for has ended: run the stop hooks
	Block   *Block
	State   GuardState
	Changed bool // State must be saved
}

// neverBlocked are apps blocking would only get in the way of.
var neverBlocked = []string{"com.apple.finder", "com.apple.loginwindow", "com.apple.dock",
	"com.apple.systempreferences", "com.tock.bar", "com.tock.trackd"}

// GuardStep decides the session hooks and blocks of one tick. A session
// whose planned end has passed counts as ended, as Step ends it in the same
// tick.
func GuardStep(in Input, gs GuardState) Guard {
	g := Guard{State: gs}
	s := openSession(in)
	if s != nil && in.Now >= s.PlannedMS {
		s = nil
	}
	switch {
	case s != nil && gs.HookSession != s.ID:
		cp := *s
		g.Start = &cp
		g.Stop = gs.HookSession != 0 // a session that ended unseen, then a new one
		g.State.HookSession, g.Changed = s.ID, true
	case s == nil && gs.HookSession != 0:
		g.Stop = true
		g.State.HookSession, g.Changed = 0, true
	}
	if s == nil || !in.Cfg.Block || inMeeting(in) {
		return g
	}
	n := len(in.Views)
	c := current(in)
	if c == nil || in.Now-c.Start < in.Cfg.BlockGraceMS || in.Now-gs.BlockAt < in.Cfg.BlockGraceMS {
		return g
	}
	if !Blockable(*c, in.Labels[n-1], in.Cfg.BlockApps, in.Cfg.BlockHosts) {
		return g
	}
	g.Block = &Block{Bundle: c.BundleID, App: c.App, Host: Host(*c), LeftMS: s.PlannedMS - in.Now}
	g.State.BlockAt, g.Changed = in.Now, true
	return g
}

// Blockable reports a view the guard removes: a distraction, an app in
// apps, or a URL whose host or a parent domain is in hosts. Time tock
// makes up (idle, away, calls, meetings, other machines) and system apps
// never are.
func Blockable(v classify.SpanView, l classify.Label, apps, hosts []string) bool {
	if strings.HasPrefix(v.BundleID, "tock.") || classify.IsRemote(v) || slices.Contains(neverBlocked, v.BundleID) {
		return false
	}
	if l.Kind == classify.KindDistract || slices.Contains(apps, v.BundleID) {
		return true
	}
	h := Host(v)
	for _, b := range hosts {
		if h != "" && (h == b || strings.HasSuffix(h, "."+b)) {
			return true
		}
	}
	return false
}

// Host is the URL host of v without its port, or "".
func Host(v classify.SpanView) string {
	if v.URL == nil {
		return ""
	}
	h, _, _ := strings.Cut(strings.ToLower(*v.URL), "/")
	h, _, _ = strings.Cut(h, ":")
	return h
}
