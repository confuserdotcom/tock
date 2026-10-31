package focus

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"tock/internal/classify"
	"tock/internal/stats"
)

func running() Session {
	return Session{ID: 7, Kind: KindManual, StartMS: at(14, 0, 0), PlannedMS: at(14, 25, 0)}
}

func TestSessionFirstAlert(t *testing.T) {
	in := input(at(14, 6, 10),
		vw{at(14, 0, 0), at(14, 5, 0), classify.KindDeep, "", ""},
		vw{at(14, 5, 0), at(14, 6, 10), classify.KindDistract, "", ""})
	in.Sessions = []Session{running()}
	upd := running()
	upd.Interruptions, upd.AlertedMS = 1, 70_000
	upd.DeepMS, upd.DistractMS, upd.LongestMS = 5*minute, 70_000, 5*minute
	want := []Event{
		{Kind: SessionUpdate, Sess: upd},
		{Kind: Notify, Topic: TopicFocus, Title: TitleFocus, Body: "1 min distracted in your 25 min session, 19 min left.", OfSession: true, Sess: upd},
	}
	if got := Step(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("Step =\n%+v\nwant\n%+v", got, want)
	}
}

func TestSessionGraceNotReached(t *testing.T) {
	in := input(at(14, 5, 50),
		vw{at(14, 0, 0), at(14, 5, 0), classify.KindDeep, "", ""},
		vw{at(14, 5, 0), at(14, 5, 50), classify.KindDistract, "", ""})
	in.Sessions = []Session{running()}
	upd := running()
	upd.DeepMS, upd.DistractMS, upd.LongestMS = 5*minute, 50_000, 5*minute
	if got, want := Step(in), []Event{{Kind: SessionUpdate, Sess: upd}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Step = %+v, want %+v", got, want)
	}
	// Nothing changed since: no write at all.
	in.Sessions = []Session{upd}
	if got := Step(in); len(got) != 0 {
		t.Fatalf("unchanged tick = %+v, want nothing", got)
	}
}

func TestSessionRepeatAlert(t *testing.T) {
	views := func(distractEnd int64) Input {
		in := input(distractEnd,
			vw{at(14, 0, 0), at(14, 5, 0), classify.KindDeep, "", ""},
			vw{at(14, 5, 0), at(14, 6, 10), classify.KindDistract, "", ""},
			vw{at(14, 6, 10), at(14, 10, 0), classify.KindDeep, "", ""},
			vw{at(14, 10, 0), distractEnd, classify.KindDistract, "", ""})
		s := running()
		s.Interruptions, s.AlertedMS = 1, 70_000
		in.Sessions = []Session{s}
		return in
	}
	// 70 s + 110 s = 180 s: 10 s short of the 120 s repeat.
	if notes := only(Step(views(at(14, 11, 50))), Notify); len(notes) != 0 {
		t.Fatalf("before the repeat: %+v", notes)
	}
	evs := Step(views(at(14, 12, 0)))
	notes := only(evs, Notify)
	if len(notes) != 1 || notes[0].Body != "3 min distracted in your 25 min session, 13 min left." {
		t.Fatalf("repeat notes = %+v", notes)
	}
	upd := only(evs, SessionUpdate)
	if len(upd) != 1 || upd[0].Sess.Interruptions != 2 || upd[0].Sess.AlertedMS != 190_000 {
		t.Fatalf("repeat update = %+v", upd)
	}
}

func TestSessionMeetingMutes(t *testing.T) {
	in := input(at(14, 6, 10),
		vw{at(14, 0, 0), at(14, 5, 0), classify.KindDeep, "", ""},
		vw{at(14, 5, 0), at(14, 6, 10), classify.KindDistract, "", ""})
	in.Sessions = []Session{running()}
	in.Call = true
	evs := Step(in)
	if len(only(evs, Notify)) != 0 {
		t.Fatalf("camera on: %+v", evs)
	}
	if u := only(evs, SessionUpdate); len(u) != 1 || u[0].Sess.Interruptions != 0 {
		t.Fatalf("camera on update = %+v", u)
	}

	// Distraction-labelled time in a meeting tab is not distraction.
	in = input(at(14, 7, 0),
		vw{at(14, 0, 0), at(14, 5, 0), classify.KindDeep, "", ""},
		vw{at(14, 5, 0), at(14, 7, 0), classify.KindDistract, "arc", "meet.google.com/abc"})
	in.Sessions = []Session{running()}
	if notes := only(Step(in), Notify); len(notes) != 0 {
		t.Fatalf("meeting tab: %+v", notes)
	}
}

// endFixture is a 25 min session whose first 20 min are deep and last 5 min
// distracted, with the given category names and a project on the deep part.
func endFixture(deepCat, distractCat, project string) (Input, Session) {
	in := input(at(14, 26, 0),
		vw{at(14, 0, 0), at(14, 20, 0), classify.KindDeep, "", ""},
		vw{at(14, 20, 0), at(14, 26, 0), classify.KindDistract, "", ""})
	in.Labels[0].Category, in.Labels[0].Project = deepCat, project
	in.Labels[1].Category = distractCat
	in.Sessions = []Session{running()}
	end := running()
	end.EndMS = at(14, 25, 0)
	end.DeepMS, end.DistractMS, end.LongestMS = 20*minute, 5*minute, 20*minute
	end.Switches = stats.WindowSwitches(in.Views, in.Labels, end.StartMS, end.PlannedMS)
	return in, end
}

func TestSessionEnd(t *testing.T) {
	in, end := endFixture("Code", "Video", "tock")
	if end.Switches != 1 {
		t.Fatalf("fixture switches = %d, want 1", end.Switches)
	}
	q := Quality(end, Weights{})
	want := []Event{
		{Kind: SessionEnd, Sess: end},
		{Kind: StateUpdate, State: State{BreakFrom: at(14, 0, 0), BreakAt: at(14, 26, 0)}},
		{Kind: Notify, Topic: TopicDone, Title: TitleDone, OfSession: true, Minutes: 25, Sess: end,
			Project: "tock", Refile: true,
			Body: fmt.Sprintf("25 min · quality %d · 80%% deep · no interruptions · top: Code, Video", q.Score)},
	}
	if got := Step(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("Step =\n%+v\nwant\n%+v", got, want)
	}
	b := Window(in.Views, in.Labels, end.StartMS, end.PlannedMS, in.Cfg)
	if b.Project != "tock" || !b.Refile() {
		t.Fatalf("Window = %+v, want project tock with a refile", b)
	}
}

func TestSessionEndNoTop(t *testing.T) {
	in, end := endFixture("", "", "")
	q := Quality(end, Weights{})
	done := only(Step(in), Notify)[0]
	if want := fmt.Sprintf("25 min · quality %d · 80%% deep · no interruptions", q.Score); done.Body != want {
		t.Fatalf("Body = %q, want %q", done.Body, want)
	}
	if done.Project != "" || done.Refile {
		t.Fatalf("Project = %q Refile = %v, want none", done.Project, done.Refile)
	}
}

func TestSessionEndLongNames(t *testing.T) {
	long := strings.Repeat("é", 25)
	wide := strings.Repeat("漢", 15) // 30 cells
	in, _ := endFixture(long, wide, "")
	body := only(Step(in), Notify)[0].Body
	want := " · top: " + strings.Repeat("é", 19) + "…, " + strings.Repeat("漢", 9) + "…"
	if !strings.HasSuffix(body, want) {
		t.Fatalf("Body = %q, want the suffix %q", body, want)
	}
	for _, name := range strings.Split(strings.TrimPrefix(body[strings.Index(body, " · top: "):], " · top: "), ", ") {
		if w := ansi.StringWidth(name); w > 20 {
			t.Fatalf("name %q is %d cells wide, want at most 20", name, w)
		}
	}
}
