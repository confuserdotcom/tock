package app

import (
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/focus"
)

const minMS = int64(60_000)

// insertSession adds a finished manual session that started at hhmm, day days
// from today, planned for min minutes. rating 0 = unrated.
func insertSession(t *testing.T, e *apptest.Env, hhmm string, day int, min int64, rating int) {
	t.Helper()
	db := e.RawDB(t)
	defer db.Close()
	start := apptest.At(t, hhmm, day)
	var r, rms any
	if rating > 0 {
		r, rms = rating, start+min*minMS
	}
	if _, err := db.Exec(`INSERT INTO session (kind, start_ms, planned_ms, end_ms, rating, rated_ms)
		VALUES ('manual', ?, ?, ?, ?, ?)`, start, start+min*minMS, start+min*minMS, r, rms); err != nil {
		t.Fatal(err)
	}
}

func TestProfileSessions(t *testing.T) {
	e := recEnv(t, "")
	for i := 1; i <= 5; i++ {
		insertSession(t, e, "13:00", -i, 45, 4)
	}
	insertSession(t, e, "15:00", -1, 20, 0)
	insertSession(t, e, "15:00", -2, 20, 0)
	a := open(t, e)
	p, err := a.Profile(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if p.SessionMS != 45*minMS || p.GoodSessions != 5 {
		t.Fatalf("SessionMS %d min, GoodSessions %d; want 45, 5", p.SessionMS/minMS, p.GoodSessions)
	}
	evs, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	wantList(t, "09:30", notes(evs), wantNote(focus.TopicRecommend, "Good time to focus",
		"Your best focus hour · start a 45 min session now?"))
	if evs[0].Minutes != 45 {
		t.Errorf("Minutes = %d, want 45", evs[0].Minutes)
	}
}

func TestProfileFewSessionsKeepDefault(t *testing.T) {
	e := recEnv(t, "")
	for i := 1; i <= 4; i++ {
		insertSession(t, e, "13:00", -i, 45, 5)
	}
	p, err := open(t, e).Profile(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if p.SessionMS != 25*minMS || p.GoodSessions != 0 {
		t.Fatalf("SessionMS %d min, GoodSessions %d; want the default 25, 0", p.SessionMS/minMS, p.GoodSessions)
	}
}

func TestProfileSustained(t *testing.T) {
	e := recEnv(t, "")
	p, err := open(t, e).Profile(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if p.SustainedMS != 60*minMS || p.SustainedDays != 21 {
		t.Fatalf("SustainedMS %d min, SustainedDays %d; want 60, 21", p.SustainedMS/minMS, p.SustainedDays)
	}

	e = apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.FocusHistory(6)...)
	e.WriteConfig(t, apptest.NoRoutines+apptest.NoPlanNotices+apptest.AllHours)
	slotClock(e, 9, 30, 0)
	p, err = open(t, e).Profile(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if p.SustainedMS != 0 || p.SustainedDays != 6 {
		t.Fatalf("6 days: SustainedMS %d, SustainedDays %d; want 0, 6", p.SustainedMS, p.SustainedDays)
	}
}

// TestProfileTopHoursInsideWorkWindow: the profile ranks the hours of the
// [hours] window, so history in 09:00-10:00 gives no top hour for a 10:00-18:00
// schedule.
func TestProfileTopHoursInsideWorkWindow(t *testing.T) {
	late := "[hours]\ndays = [\"mon\", \"tue\", \"wed\", \"thu\", \"fri\", \"sat\", \"sun\"]\nstart = \"10:00\"\nend = \"18:00\"\n"
	e := recEnvCfg(t, apptest.NoRoutines+apptest.NoPlanNotices+late)
	p, err := open(t, e).Profile(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if p.Rank(time.Saturday, 9) != 0 || len(p.Top[time.Saturday]) != 0 {
		t.Errorf("Top[Saturday] = %v, want none outside a 10:00 start", p.Top[time.Saturday])
	}
	if p.Hours[time.Saturday][9].Samples == 0 {
		t.Error("the grid lost the hour outside the window")
	}
	e = recEnv(t, "")
	if p, err = open(t, e).Profile(e.Clock); err != nil || p.Rank(time.Saturday, 9) != 1 {
		t.Errorf("08:00 start: rank of 9 = %d, err %v; want 1", p.Rank(time.Saturday, 9), err)
	}
}
