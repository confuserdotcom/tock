package agentlog

import (
	"slices"
	"strings"

	"tock/internal/classify"
)

// Around is how far before its first and after its last activity a
// session's window may be in front and still be that session.
const (
	AroundBefore = 10 * 60_000
	AroundAfter  = 30 * 60_000
)

// Attach sets the working directory of the sessions on terminal views that
// have none: a view whose title holds a session's title (coding agents put
// it in the terminal title), in front around the time the session ran.
// When several fit, the session with activity nearest the view wins. Dirs
// under home are written "~/…", as the shell hook writes them.
func Attach(views []classify.SpanView, sessions []Session, terminals []string, home string) {
	if len(sessions) == 0 {
		return
	}
	for i := range views {
		v := &views[i]
		if v.Dir != nil || v.Private || v.Title == nil || !slices.Contains(terminals, v.BundleID) {
			continue
		}
		title := classify.Clean(*v.Title)
		best, bestGap := -1, int64(1<<62)
		for j, s := range sessions {
			if s.Dir == "" || len(s.Events) == 0 || !holdsTitle(title, s.Titles) {
				continue
			}
			first, last := s.Events[0], s.Events[len(s.Events)-1]
			if v.End < first-AroundBefore || v.Start > last+AroundAfter {
				continue
			}
			if g := nearest(s.Events, v.Start, v.End); g < bestGap {
				best, bestGap = j, g
			}
		}
		if best >= 0 {
			d := fold(sessions[best].Dir, home)
			v.Dir = &d
		}
	}
}

func holdsTitle(title string, titles []string) bool {
	for _, t := range titles {
		if len([]rune(t)) >= 4 && strings.Contains(title, t) {
			return true
		}
	}
	return false
}

// nearest is how far the event nearest [from, to] is from it, 0 inside.
func nearest(events []int64, from, to int64) int64 {
	i, _ := slices.BinarySearch(events, from)
	g := int64(1 << 62)
	if i < len(events) {
		g = max(events[i]-to, 0)
	}
	if i > 0 {
		g = min(g, from-events[i-1])
	}
	return g
}

func fold(dir, home string) string {
	home = strings.TrimRight(home, "/")
	switch {
	case home == "":
		return dir
	case dir == home:
		return "~"
	case strings.HasPrefix(dir, home+"/"):
		return "~" + dir[len(home):]
	}
	return dir
}
