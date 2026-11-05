package bar

import (
	"strings"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/stats"
	"tock/internal/store"
)

var islandLoc = time.UTC

const t1720 = int64(1_793_899_200_000) // a Thursday, 17:20 UTC

func TestIslandQuiet(t *testing.T) {
	s := Snapshot{OK: true, TrackedMS: 4*3600_000 + 12*60_000, Score: 78, Active: true, UnlabelledMS: 85 * 60_000, Loc: islandLoc}
	i := s.IslandBase(t1720)
	if i.State != IslandQuiet || i.Open || i.Face != (Face{}) {
		t.Fatalf("quiet = %+v", i)
	}
	h := i.Hover
	if h == nil || h.Face.Left != "4h12" || h.Face.Right != "78" || h.Face.RightDim != "1h25 to label" {
		t.Fatalf("hover = %+v", h)
	}
	if len(h.Buttons) != 2 || h.Buttons[0].ID != IDDashboard || !h.Buttons[0].Primary || h.Buttons[1].ID != IDNotchLabel {
		t.Fatalf("buttons = %+v", h.Buttons)
	}
	s.UnlabelledMS = 30_000
	if b := s.IslandBase(t1720).Hover.Buttons; len(b) != 1 {
		t.Fatalf("under a minute unlabelled still offers labelling: %+v", b)
	}
	if i := (Snapshot{}).IslandBase(t1720); i.State != IslandQuiet || i.Hover != nil {
		t.Fatalf("not ready = %+v", i)
	}
}

func TestIslandFocusAndDrift(t *testing.T) {
	ss := &focus.Session{ID: 7, StartMS: t1720 - 6*60_000, PlannedMS: t1720 + 19*60_000, Goal: "tock notch"}
	s := Snapshot{OK: true, Session: ss, Live: focus.Live{Level: 82}, Loc: islandLoc}
	i := s.IslandBase(t1720)
	if i.State != IslandFocus || i.Face.Dot != "deep" || i.Face.Left != "tock notch" || i.Face.Right != "19:00" || i.Face.EndMS != ss.PlannedMS {
		t.Fatalf("focus = %+v", i)
	}
	if i.Hover.Sub != "17:14 to 17:39 · focus 82%" || i.Hover.Face.RightDim != "left" || i.Hover.Buttons[0].ID != IDStop {
		t.Fatalf("hover = %+v", i.Hover)
	}
	s.Live.Level = 30
	if d := s.IslandBase(t1720).Face.Dot; d != "distract" {
		t.Fatalf("low level dot = %q", d)
	}
	s.DistractAtMS = t1720 - 60_000
	if i := s.IslandBase(t1720); i.State != IslandDrift || i.Face.Left != "Off task" || i.Face.Dot != "distract" {
		t.Fatalf("drift = %+v", i)
	}
	s.DistractAtMS = t1720 - driftShowMS
	if st := s.IslandBase(t1720).State; st != IslandFocus {
		t.Fatalf("old notice still drifts: %s", st)
	}
}

func TestLevelKind(t *testing.T) {
	for l, want := range map[int]string{-1: "neutral", 0: "distract", 39: "distract", 40: "shallow", 69: "shallow", 70: "deep", 100: "deep"} {
		if got := LevelKind(l); got != want {
			t.Errorf("LevelKind(%d) = %q, want %q", l, got, want)
		}
	}
}

func TestIslandBreakAndMeet(t *testing.T) {
	s := Snapshot{OK: true, BreakUntil: time.UnixMilli(t1720 + 5*60_000), Loc: islandLoc}
	if i := s.IslandBase(t1720); i.State != IslandBreak || i.Face.Dot != DotBreak || i.Face.Right != "5:00" || i.Hover.Sub != "until 17:25" {
		t.Fatalf("break = %+v", i)
	}
	s = Snapshot{OK: true, Loc: islandLoc, Slots: []focus.Slot{
		{Kind: focus.SlotEvent, StartMS: t1720 + 2*60_000, EndMS: t1720 + 15*60_000, Title: "Standup", Calendar: "Work", JoinURL: "https://meet.google.com/abc-defg-hij"},
	}}
	i := s.IslandBase(t1720)
	if i.State != IslandMeet || i.Face.Left != "Standup" || i.Face.RightDim != "in" || i.Face.Right != "2m" {
		t.Fatalf("meet = %+v", i)
	}
	if i.Hover.Sub != "17:22 to 17:35 · Work" || len(i.Hover.Buttons) != 1 || i.Hover.Buttons[0].ID != IDJoin+"1793899320000" {
		t.Fatalf("meet hover = %+v", i.Hover)
	}
	// a meeting soon wins over the running session
	s.Session = &focus.Session{StartMS: t1720 - 60_000, PlannedMS: t1720 + 60_000}
	if st := s.IslandBase(t1720).State; st != IslandMeet {
		t.Fatalf("session over meeting: %s", st)
	}
	if _, when := startsIn(t1720-30_000, t1720); when != "now" {
		t.Fatalf("started = %q", when)
	}
}

func TestDriftAndMeetMoments(t *testing.T) {
	n := store.Notice{ID: 11, Topic: focus.TopicFocus, Title: "Back to tock notch?", Body: "YouTube for 2 min"}
	i := DriftIsland(n, app.NoticeData{Session: 7, SnoozeMin: 15, Category: "Video"})
	if !i.Open || i.Notice != 11 || i.AutoClose != driftAutoClose || i.Face.Left != "Video" || i.Panel.Lead != n.Title {
		t.Fatalf("drift = %+v", i)
	}
	b := i.Panel.Buttons
	if len(b) == 0 || !b[0].Primary || b[0].ID != "notch:act:11:"+ActSnoozeDistract || b[len(b)-1].ID != "notch:act:11:"+ActStop {
		t.Fatalf("drift buttons = %+v", b)
	}
	m := MeetIsland(store.Notice{ID: 12, Topic: focus.TopicStarting, Title: "Standup"}, app.NoticeData{Join: "https://meet.google.com/abc-defg-hij"})
	if m.Face.Right != "now" || m.Panel.Buttons[0].ID != "notch:act:12:"+ActJoin || m.Panel.Buttons[1].ID != IDNotchClose {
		t.Fatalf("meet = %+v", m.Panel.Buttons)
	}
}

func TestDoneIsland(t *testing.T) {
	n := store.Notice{ID: 9, Topic: focus.TopicDone, Title: "Session done", Body: "25 min"}
	d := app.NoticeData{Session: 7, Minutes: 25}
	det := &report.SessionDetail{Sess: focus.Session{ID: 7, StartMS: t1720, PlannedMS: t1720 + 25*60_000, EndMS: t1720 + 25*60_000,
		DeepMS: 21 * 60_000, LongestMS: 14 * 60_000, Switches: 3, DistractMS: 2 * 60_000, Goal: "tock notch"}}
	i := DoneIsland(n, d, det, islandLoc, 85*60_000)
	p := i.Panel
	if i.Haptic != HapticLevel || i.Face.Right != "25m" || p.Lead != "tock notch" || p.Sub != "17:20 to 17:45 · ended on time" {
		t.Fatalf("done = %+v / %+v", i, p)
	}
	if len(p.Cells) != 4 || p.Cells[0].V != "84%" || p.Cells[2].V != "3" || p.Cells[3].V != "2m" {
		t.Fatalf("cells = %+v", p.Cells)
	}
	var ids []string
	for _, b := range p.Buttons {
		ids = append(ids, b.ID)
	}
	if got := strings.Join(ids, " "); got != "notch:act:9:again notch:label notch:close" || !p.Buttons[0].Primary {
		t.Fatalf("buttons = %s", got)
	}
	// unreadable detail: the notice's text, still a Close
	i = DoneIsland(n, app.NoticeData{}, nil, islandLoc, 0)
	if i.Panel.Lead != "Session done" || i.Panel.Sub != "25 min" || len(i.Panel.Buttons) != 1 || !i.Panel.Buttons[0].Primary {
		t.Fatalf("no detail = %+v", i.Panel)
	}
}

func TestRecapIsland(t *testing.T) {
	day := time.Date(2026, 11, 5, 0, 0, 0, 0, islandLoc)
	h := func(hh, mm int) int64 {
		return day.Add(time.Duration(hh)*time.Hour + time.Duration(mm)*time.Minute).UnixMilli()
	}
	d := report.Day{Date: day, TrackedMS: 6*3600_000 + 48*60_000,
		Score:    stats.Score{Value: 81},
		Stats:    stats.Day{FocusMS: 192 * 60_000, LongestMS: 94 * 60_000, DistractMS: 14 * 60_000},
		Projects: []report.Share{{Name: "tock", MS: 242 * 60_000}, {Name: "", MS: 75 * 60_000}, {Name: "Licence", MS: 91 * 60_000}, {Name: "x", MS: 1}},
		Timeline: []report.Row{
			{Start: h(9, 0), End: h(12, 0), Label: classify.Label{Kind: "deep"}},
			{Start: h(12, 0), End: h(13, 0), Idle: true},
			{Start: h(13, 0), End: h(19, 0), Label: classify.Label{Kind: "shallow"}},
		}}
	u := app.Usual{Days: 3, Score: 75, FocusMS: 168 * 60_000, LongestMS: 94 * 60_000, DistractMS: 23 * 60_000}
	i := RecapIsland(store.Notice{ID: 3, Topic: focus.TopicRecap}, d, u, 0, islandLoc)
	p := i.Panel
	if i.Face.Left != "Thursday" || i.Face.Right != "6h48" {
		t.Fatalf("face = %+v", i.Face)
	}
	want := []Cell{{"Score", "81", "+6 vs usual Thu", "good"}, {"Deep focus", "3h12", "+24m", "good"}, {"Longest block", "1h34", "", ""}, {"Distracting", "14m", "−9m", "good"}}
	for k := range want {
		if p.Cells[k] != want[k] {
			t.Errorf("cell %d = %+v, want %+v", k, p.Cells[k], want[k])
		}
	}
	if len(p.Rows) != 3 || p.Rows[1] != [2]string{"No project", "1h15"} {
		t.Fatalf("rows = %+v", p.Rows)
	}
	if len(p.Strip) != stripCells || p.Strip[0] != "deep" || p.Strip[len(p.Strip)-1] != "shallow" || p.Strip[17] != "none" {
		t.Fatalf("strip = %v", p.Strip)
	}
	if strings.Join(p.Axis, " ") != "09 11 13 15 17 19" {
		t.Fatalf("axis = %v", p.Axis)
	}
	if len(p.Buttons) != 2 || p.Buttons[1].Title != "Done for today" {
		t.Fatalf("buttons = %+v", p.Buttons)
	}
	// a worse day: bad tones, the minus sign
	d.Score.Value, d.Stats.DistractMS = 70, 30*60_000
	p = RecapIsland(store.Notice{}, d, u, 0, islandLoc).Panel
	if p.Cells[0].C != "−5 vs usual Thu" || p.Cells[0].Tone != "bad" || p.Cells[3].Tone != "bad" {
		t.Fatalf("worse = %+v", p.Cells)
	}
	// no usual day: no changes
	p = RecapIsland(store.Notice{}, d, app.Usual{}, 0, islandLoc).Panel
	for _, c := range p.Cells {
		if c.C != "" || c.Tone != "" {
			t.Fatalf("no usual = %+v", c)
		}
	}
}

func TestLabelIsland(t *testing.T) {
	es := []Entry{{Title: "a", MS: 42 * 60_000, Picked: -1}, {Title: "b", MS: 18 * 60_000, Picked: -1}}
	if i := LabelIsland(es, 0, ""); i.Face.Right != "1h00" || i.Panel.At != 0 || len(i.Panel.Entries) != 2 {
		t.Fatalf("label = %+v", i)
	}
	if i := LabelIsland(es, 1, HapticAlignment); i.Face.Right != "18m" || i.Haptic != HapticAlignment {
		t.Fatalf("second = %+v", i)
	}
	i := LabelIsland(es, 2, "")
	if i.Face.Right != "clear" || i.Panel.Buttons[0].ID != IDNotchClose {
		t.Fatalf("clear = %+v", i)
	}
}

func TestNotchTopicAndRank(t *testing.T) {
	for _, tp := range []string{focus.TopicDone, focus.TopicRecap, focus.TopicFocus, focus.TopicUpcoming, focus.TopicStarting} {
		if !NotchTopic(tp) {
			t.Errorf("%s not in the notch", tp)
		}
	}
	for _, tp := range []string{focus.TopicBreak, focus.TopicAway, focus.TopicWake, focus.TopicRecommend, focus.TopicBreakScreen} {
		if NotchTopic(tp) {
			t.Errorf("%s in the notch", tp)
		}
	}
	if !(Rank(IslandMeet) > Rank(IslandDrift) && Rank(IslandDrift) > Rank(IslandDone) && Rank(IslandDone) > Rank(IslandRecap) && Rank(IslandRecap) > Rank(IslandLabel)) {
		t.Fatal("rank order")
	}
}

func TestIslandJSON(t *testing.T) {
	j := LabelIsland([]Entry{{Key: "secret-key", App: "Figma", Title: "x", Dur: "42m", MS: 1, Picked: -1}}, 0, "").JSON()
	for _, want := range []string{`"state":"label"`, `"open":true`, `"app":"Figma"`, `"picked":-1`} {
		if !strings.Contains(j, want) {
			t.Errorf("json misses %s: %s", want, j)
		}
	}
	if strings.Contains(j, "secret-key") {
		t.Fatalf("key leaks to the UI: %s", j)
	}
}
