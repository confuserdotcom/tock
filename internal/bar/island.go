package bar

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/store"
)

// Island states: what the notch shows.
const (
	IslandQuiet  = "quiet"  // nothing runs: hidden behind the notch
	IslandFocus  = "focus"  // a session runs
	IslandDrift  = "drift"  // a session runs and went off task
	IslandBreak  = "break"  // a break runs
	IslandMeet   = "meet"   // a plan item or event starts soon
	IslandDone   = "done"   // a session just ended
	IslandRecap  = "recap"  // the end-of-day recap
	IslandLabel  = "label"  // labelling the inbox
	IslandBudget = "budget" // a project's budget ran low or out
)

// Notch ids: the Cocoa side sends them as menu clicks.
const (
	IDNotch      = "notch:"
	IDNotchClose = "notch:close" // the open moment was dismissed
	IDNotchLabel = "notch:label" // open labelling
	IDNotchPick  = "notch:pick:" // + "<entry>:<option>"
	IDNotchSkip  = "notch:skip:" // + "<entry>"
	IDNotchAct   = "notch:act:"  // + "<notice id>:<notification action>"
	// IDNotchExpire (+ notice id) closes that moment when its time is up.
	IDNotchExpire = "notch:expire:"
	// IDNotchMenu (+ a menu id) is a panel button that does what the menu
	// item does; it closes the moment it was on.
	IDNotchMenu = "notch:menu:"
	// IDNotchSeen is sent when a missed moment was opened and put away.
	IDNotchSeen = "notch:seen"
	// IDNotchOpen and IDNotchShut are sent when you open the quiet or
	// running panel and when it folds: the pages are read only while open.
	IDNotchOpen = "notch:open"
	IDNotchShut = "notch:shut"
)

// Dot kinds beyond the kinds of time.
const (
	DotGood  = "good"
	DotBreak = "break"
)

// Haptic patterns of NSHapticFeedbackManager.
const (
	HapticLevel     = "level"
	HapticAlignment = "alignment"
)

const (
	// driftShowMS is how long after a distraction notice the compact island
	// shows the session as off task.
	driftShowMS = 2 * 60_000
	// meetAheadMS is how far ahead a slot shows in the compact island.
	meetAheadMS = 10 * 60_000
	// meetAfterMS is how long after its start a slot still shows.
	meetAfterMS = 60_000
	// driftAutoClose is how many seconds an open drift nudge stays.
	driftAutoClose = 12
	// meetAutoClose, doneAutoClose and recapAutoClose are how many seconds
	// those moments stay open when nobody points at them. A session end and
	// the recap then wait in the quiet notch (see Missed).
	meetAutoClose  = 90
	doneAutoClose  = 30
	recapAutoClose = 60
	// labelEntries is how many inbox entries one labelling round offers.
	labelEntries = 3
	// minLabelMS is the least unlabelled time the island offers to label.
	minLabelMS = 60_000
	// recapProjects is how many projects the recap lists.
	recapProjects = 3
	// goalCells is how wide a goal or title may be in the compact island.
	goalCells = 22
)

// Island is one state of the notch: Face either side of it while compact,
// Panel when open, Hover when the pointer rests on a compact one.
type Island struct {
	State  string `json:"state"`
	Open   bool   `json:"open"`             // opens by itself: a moment, not the running state
	Notice int64  `json:"notice,omitempty"` // the notice it answers, 0 = none
	Haptic string `json:"haptic,omitempty"` // played once when it opens or a label is saved
	// AutoClose is how many seconds an open moment stays; 0 = until answered.
	AutoClose int    `json:"auto_close,omitempty"`
	Face      Face   `json:"face"`
	Panel     *Panel `json:"panel,omitempty"`
	Hover     *Panel `json:"hover,omitempty"`
	// Pending marks a quiet notch holding a moment that folded unseen: its
	// dot shows, and the panel opened from it is that moment.
	Pending bool `json:"pending,omitempty"`
}

// Face is the compact text either side of the notch.
type Face struct {
	Dot      string `json:"dot,omitempty"` // a kind of time, DotGood or DotBreak; "" = none
	Left     string `json:"left,omitempty"`
	LeftDim  string `json:"left_dim,omitempty"`
	Right    string `json:"right,omitempty"`
	RightDim string `json:"right_dim,omitempty"`
	// EndMS is a countdown target: the Cocoa side writes the countdown in
	// Right itself, so the island stays the same from second to second.
	EndMS int64 `json:"end_ms,omitempty"`
}

// Panel is the open notch below its face.
type Panel struct {
	Face    Face        `json:"face"`
	Lead    string      `json:"lead,omitempty"`
	Sub     string      `json:"sub,omitempty"`
	Cells   []Cell      `json:"cells,omitempty"`
	Strip   []string    `json:"strip,omitempty"` // kinds, left to right
	Axis    []string    `json:"axis,omitempty"`  // labels under the strip, spread evenly
	Rows    [][2]string `json:"rows,omitempty"`
	Buttons []NButton   `json:"buttons,omitempty"`
	Entries []Entry     `json:"entries,omitempty"`
	At      int         `json:"at,omitempty"` // the entry shown
	// Pages replace Lead to Rows in the open quiet notch: you swipe between
	// them, and the panel can be resized (see Page).
	Pages []Page `json:"pages,omitempty"`
}

// Cell is one figure of a number row: label, value, change and its tone.
type Cell struct {
	K    string `json:"k"`
	V    string `json:"v"`
	C    string `json:"c,omitempty"`
	Tone string `json:"tone,omitempty"` // "good", "bad" or "" (muted)
}

// NButton is a button of the panel.
type NButton struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Primary bool   `json:"primary,omitempty"`
}

// Entry is one inbox key to label.
type Entry struct {
	Key    string   `json:"-"`
	App    string   `json:"app"`
	Title  string   `json:"title"`
	Sub    string   `json:"sub,omitempty"`
	Dur    string   `json:"dur"`
	MS     int64    `json:"-"`
	Opts   []Option `json:"opts"`
	Saved  string   `json:"saved,omitempty"` // what the pick did
	Picked int      `json:"picked"`          // the option picked, -1 = none
}

// Option is a label offered for an entry.
type Option struct {
	Label string `json:"label"`          // as WantText writes it
	Kind  string `json:"kind,omitempty"` // its category's kind
	Pct   int    `json:"pct,omitempty"`  // the guess's share, 0 = not a guess
}

// JSON is the island as the Cocoa side reads it.
func (i Island) JSON() string {
	b, err := json.Marshal(i)
	if err != nil { // only plain fields: cannot happen
		return `{"state":"quiet","face":{}}`
	}
	return string(b)
}

// Rank orders moments: a higher one replaces a lower one that is open, a
// lower one waits as a banner.
func Rank(state string) int {
	switch state {
	case IslandMeet:
		return 5
	case IslandDrift:
		return 4
	case IslandDone:
		return 3
	case IslandRecap, IslandBudget:
		return 2
	case IslandLabel:
		return 1
	}
	return 0
}

// NotchTopic reports whether the notch shows notices of topic instead of a
// banner. Up-next notices need no moment: the compact island shows the slot.
func NotchTopic(topic string) bool {
	switch topic {
	case focus.TopicDone, focus.TopicRecap, focus.TopicFocus, focus.TopicUpcoming, focus.TopicStarting, focus.TopicBudget:
		return true
	}
	return false
}

// LevelKind is the kind colour of a live focus level.
func LevelKind(level int) string {
	switch {
	case level < 0:
		return "neutral"
	case level >= 70:
		return "deep"
	case level >= 40:
		return "shallow"
	}
	return "distract"
}

func (s Snapshot) loc() *time.Location {
	if s.Loc != nil {
		return s.Loc
	}
	return time.Local
}

func clock(ms int64, loc *time.Location) string { return time.UnixMilli(ms).In(loc).Format("15:04") }

// IslandBase is what the notch shows at nowMS with no moment open.
func (s Snapshot) IslandBase(nowMS int64) Island {
	if !s.OK {
		return Island{State: IslandQuiet}
	}
	if sl, ok := s.soonSlot(nowMS); ok {
		return s.meetIsland(sl, nowMS)
	}
	if ss := s.Session; ss != nil {
		return s.focusIsland(*ss, nowMS)
	}
	if !s.BreakUntil.IsZero() && s.BreakUntil.UnixMilli() > nowMS {
		end := s.BreakUntil.UnixMilli()
		return Island{State: IslandBreak,
			Face: Face{Dot: DotBreak, Left: "Break", Right: report.Countdown(end - nowMS), RightDim: "left", EndMS: end},
			Hover: &Panel{Face: Face{Dot: DotBreak, Left: "Break", Right: report.Countdown(end - nowMS), RightDim: "left", EndMS: end},
				Lead: "Break", Sub: "until " + clock(end, s.loc())}}
	}
	return s.quietIsland()
}

func (s Snapshot) quietIsland() Island {
	f := Face{Left: report.Dur(s.TrackedMS), LeftDim: "today"}
	if s.Active {
		f.Right = strconv.Itoa(s.Score)
	}
	p := &Panel{Face: f, Buttons: []NButton{{ID: IDDashboard, Title: "Open Today", Primary: true}}}
	if s.UnlabelledMS >= minLabelMS {
		f.RightDim = report.Dur(s.UnlabelledMS) + " to label"
		p.Face = f
		p.Sub = report.Dur(s.UnlabelledMS) + " of today has no label yet."
		p.Buttons = append(p.Buttons, NButton{ID: IDNotchLabel, Title: "Label " + report.Dur(s.UnlabelledMS)})
	} else if s.Active {
		p.Sub = "Everything today has a label."
	}
	return Island{State: IslandQuiet, Hover: p}
}

func (s Snapshot) focusIsland(ss focus.Session, nowMS int64) Island {
	goal := ss.Goal
	if goal == "" {
		goal = "Focus"
	}
	goal = report.Cut(goal, goalCells)
	f := Face{Dot: LevelKind(s.Live.Level), Left: goal}
	if ss.PlannedMS > 0 {
		// "left": alone, 18:58 reads as the time of day
		f.EndMS, f.RightDim = ss.PlannedMS, "left"
	}
	state := IslandFocus
	if s.DistractAtMS > 0 && nowMS-s.DistractAtMS < driftShowMS {
		state, f.Dot, f.Left = IslandDrift, "distract", "Off task"
	}
	sub := "since " + clock(ss.StartMS, s.loc())
	if ss.PlannedMS > 0 {
		sub = clock(ss.StartMS, s.loc()) + " to " + clock(ss.PlannedMS, s.loc())
	}
	if s.Live.Level >= 0 {
		sub += fmt.Sprintf(" · focus %d%%", s.Live.Level)
	}
	hf := f
	lead := ss.Goal
	if lead == "" {
		lead = "Focus session"
	}
	return Island{State: state, Face: f,
		Hover: &Panel{Face: hf, Lead: lead, Sub: sub, Buttons: []NButton{{ID: IDStop, Title: "Stop session"}}}}
}

// soonSlot is the plan item or event that starts within meetAheadMS of nowMS
// or started under meetAfterMS ago.
func (s Snapshot) soonSlot(nowMS int64) (focus.Slot, bool) {
	for _, sl := range s.Slots {
		if sl.StartMS <= nowMS+meetAheadMS && sl.StartMS > nowMS-meetAfterMS {
			return sl, true
		}
	}
	return focus.Slot{}, false
}

func startsIn(startMS, nowMS int64) (string, string) {
	m := (startMS - nowMS + 59_999) / 60_000
	if m <= 0 {
		return "", "now"
	}
	return "in", strconv.FormatInt(m, 10) + "m"
}

func (s Snapshot) meetIsland(sl focus.Slot, nowMS int64) Island {
	dim, when := startsIn(sl.StartMS, nowMS)
	f := Face{Dot: "shallow", Left: report.Cut(sl.Title, goalCells), RightDim: dim, Right: when}
	sub := clock(sl.StartMS, s.loc()) + " to " + clock(sl.EndMS, s.loc())
	if sl.Calendar != "" {
		sub += " · " + sl.Calendar
	}
	p := &Panel{Face: f, Lead: sl.Title, Sub: sub}
	if j, ok := s.joinSlot(nowMS); ok && j.StartMS == sl.StartMS {
		p.Buttons = []NButton{{ID: IDJoin + strconv.FormatInt(sl.StartMS, 10), Title: "Join", Primary: true}}
	}
	return Island{State: IslandMeet, Face: f, Hover: p}
}

// noticeButtons are a notice's actions as notch buttons, the first primary.
func noticeButtons(n store.Notice, d app.NoticeData, keep func(Button) bool) []NButton {
	var out []NButton
	for _, b := range Buttons(n, d) {
		if keep != nil && !keep(b) {
			continue
		}
		out = append(out, NButton{ID: IDNotchAct + strconv.FormatInt(n.ID, 10) + ":" + b.ID, Title: b.Title})
	}
	if len(out) > 0 {
		out[0].Primary = true
	}
	return out
}

var closeButton = NButton{ID: IDNotchClose, Title: "Close"}

// DriftIsland is the nudge of a distraction notice.
func DriftIsland(n store.Notice, d app.NoticeData) Island {
	left := "Off task"
	if d.Category != "" {
		left = report.Cut(d.Category, goalCells)
	}
	f := Face{Dot: "distract", Left: left, RightDim: "off task"}
	return Island{State: IslandDrift, Open: true, Notice: n.ID, AutoClose: driftAutoClose, Face: f,
		Panel: &Panel{Face: f, Lead: n.Title, Sub: n.Body, Buttons: noticeButtons(n, d, nil)}}
}

// MeetIsland is the moment of a starting plan item or event.
func MeetIsland(n store.Notice, d app.NoticeData) Island {
	f := Face{Dot: "shallow", Left: report.Cut(n.Title, goalCells), Right: "now"}
	return Island{State: IslandMeet, Open: true, Notice: n.ID, AutoClose: meetAutoClose, Face: f,
		Panel: &Panel{Face: f, Lead: n.Title, Sub: n.Body, Buttons: append(noticeButtons(n, d, nil), closeButton)}}
}

// DoneIsland is the end of a session: detail is nil when it could not be
// read, and the notice's own text stands in.
func DoneIsland(n store.Notice, d app.NoticeData, detail *report.SessionDetail, loc *time.Location, unlabelledMS int64) Island {
	f := Face{Dot: DotGood, Left: "Session done"}
	p := &Panel{Lead: n.Title, Sub: n.Body}
	if detail != nil {
		ss := detail.Sess
		f.Right = report.Dur(ss.EndMS - ss.StartMS)
		if ss.Goal != "" {
			p.Lead = ss.Goal
		}
		p.Sub = clock(ss.StartMS, loc) + " to " + clock(ss.EndMS, loc)
		if ss.PlannedMS > 0 && ss.EndMS >= ss.PlannedMS-60_000 {
			p.Sub += " · ended on time"
		}
		p.Cells = []Cell{
			{K: "Deep focus", V: strconv.Itoa(detail.DeepPct()) + "%"},
			{K: "Longest block", V: report.Dur(ss.LongestMS)},
		}
		if ss.Switches >= 0 {
			p.Cells = append(p.Cells, Cell{K: "Switches", V: strconv.Itoa(ss.Switches)})
		}
		p.Cells = append(p.Cells, Cell{K: "Distracting", V: report.Dur(ss.DistractMS)})
	}
	p.Face = f
	p.Buttons = noticeButtons(n, d, func(b Button) bool { return b.ID == ActAgain || b.ID == ActRefile })
	if unlabelledMS >= minLabelMS {
		p.Buttons = append(p.Buttons, NButton{ID: IDNotchLabel, Title: "Label " + report.Dur(unlabelledMS)})
	}
	p.Buttons = append(p.Buttons, closeButton)
	p.Buttons[0].Primary = true
	return Island{State: IslandDone, Open: true, Notice: n.ID, Haptic: HapticLevel, AutoClose: doneAutoClose, Face: f, Panel: p}
}

// Missed is base with moment m waiting in it: a quiet notch shows a dot
// and opens m's panel. A running session, break or slot keeps its face.
func Missed(base, m Island) Island {
	if base.State != IslandQuiet || m.Panel == nil {
		return base
	}
	dot := m.Face.Dot
	if dot == "" {
		dot = "deep"
	}
	return Island{State: IslandQuiet, Face: Face{Dot: dot}, Hover: m.Panel, Pending: true}
}

// change is a cell's change against the usual day: less is better when
// lessGood. Empty with no usual day.
func change(ms, usual int64, has, lessGood bool) (string, string) {
	if !has || ms == usual {
		return "", ""
	}
	tone := "good"
	if (ms < usual) != lessGood {
		tone = "bad"
	}
	return report.Delta(ms - usual), tone
}

// RecapIsland is the end-of-day recap of d against the usual day u.
func RecapIsland(n store.Notice, d report.Day, u app.Usual, unlabelledMS int64, loc *time.Location) Island {
	f := Face{Left: d.Date.Format("Monday"), Right: report.Dur(d.TrackedMS), RightDim: "tracked"}
	has := u.Days > 0
	score := Cell{K: "Score", V: strconv.Itoa(d.Score.Value)}
	if has && d.Score.Value != u.Score {
		diff := d.Score.Value - u.Score
		score.C, score.Tone = fmt.Sprintf("%+d vs usual %s", diff, d.Date.Format("Mon")), "good"
		if diff < 0 {
			score.C, score.Tone = strings.Replace(score.C, "-", "−", 1), "bad"
		}
	}
	cell := func(k string, ms, usual int64, lessGood bool) Cell {
		c, t := change(ms, usual, has, lessGood)
		return Cell{K: k, V: report.Dur(ms), C: c, Tone: t}
	}
	p := &Panel{Face: f, Cells: []Cell{score,
		cell("Deep focus", d.Stats.FocusMS, u.FocusMS, false),
		cell("Longest block", d.Stats.LongestMS, u.LongestMS, false),
		cell("Distracting", d.Stats.DistractMS, u.DistractMS, true)}}
	p.Strip, p.Axis = dayStrip(d, loc)
	for i, sh := range d.Projects {
		if i == recapProjects {
			break
		}
		name := sh.Name
		if name == "" {
			name = "No project"
		}
		p.Rows = append(p.Rows, [2]string{report.Cut(name, goalCells), report.Dur(sh.MS)})
	}
	p.Buttons = []NButton{{ID: IDDashboard, Title: "Open Today", Primary: true}}
	if unlabelledMS >= minLabelMS {
		p.Buttons = append(p.Buttons, NButton{ID: IDNotchLabel, Title: "Label " + report.Dur(unlabelledMS)})
	}
	p.Buttons = append(p.Buttons, NButton{ID: IDNotchClose, Title: "Done for today"})
	return Island{State: IslandRecap, Open: true, Notice: n.ID, AutoClose: recapAutoClose, Face: f, Panel: p}
}

// stripCells is how many blocks the recap's day strip has.
const stripCells = 48

// dayStrip is the day's timeline as stripCells kinds from its first to its
// last activity, each block the kind with the most time in it ("none" for
// idle or nothing), and hour labels under it.
func dayStrip(d report.Day, loc *time.Location) ([]string, []string) {
	var from, to int64
	for _, r := range d.Timeline {
		if r.Idle {
			continue
		}
		if from == 0 || r.Start < from {
			from = r.Start
		}
		to = max(to, r.End)
	}
	if to <= from {
		return nil, nil
	}
	step := (to - from + stripCells - 1) / stripCells
	cells := make([]map[string]int64, stripCells)
	for _, r := range d.Timeline {
		if r.Idle {
			continue
		}
		kind := r.Label.Kind
		if kind == "" {
			kind = "none"
		}
		for c := max(0, int((r.Start-from)/step)); c < stripCells; c++ {
			lo, hi := from+int64(c)*step, from+int64(c+1)*step
			if lo >= r.End {
				break
			}
			if cells[c] == nil {
				cells[c] = map[string]int64{}
			}
			cells[c][kind] += min(hi, r.End) - max(lo, r.Start)
		}
	}
	out := make([]string, stripCells)
	for c, m := range cells {
		out[c] = "none"
		var best int64
		for _, k := range []string{"deep", "shallow", "browse", "neutral", "distract", "none"} {
			if m[k] > best {
				out[c], best = k, m[k]
			}
		}
	}
	h0 := time.UnixMilli(from).In(loc)
	h1 := time.UnixMilli(to).In(loc)
	var axis []string
	first, last := h0.Hour(), h1.Hour()+1
	every := max(1, (last-first+5)/6)
	for h := first; h <= last; h += every {
		axis = append(axis, fmt.Sprintf("%02d", h%24))
	}
	return out, axis
}

// LabelIsland is labelling entries, showing entry at.
func LabelIsland(entries []Entry, at int, haptic string) Island {
	var leftMS int64
	for i := at; i < len(entries); i++ {
		leftMS += entries[i].MS
	}
	f := Face{Left: "To label", Right: report.Dur(leftMS), RightDim: "left"}
	p := &Panel{Face: f, Entries: entries, At: at}
	if at >= len(entries) {
		f = Face{Dot: DotGood, Left: "Inbox", Right: "clear"}
		p.Face = f
		p.Lead = "Nothing left to label"
		p.Sub = "New rules apply to past days too."
		p.Buttons = []NButton{{ID: IDNotchClose, Title: "Close", Primary: true}}
	}
	return Island{State: IslandLabel, Open: true, Haptic: haptic, Face: f, Panel: p}
}
