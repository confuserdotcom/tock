package app_test

import (
	"path/filepath"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/apptest"
	"tock/internal/snapshot"
	"tock/internal/store"
)

// openApp opens e, seeding one span first: app.Open wants a database
// trackd has written to.
func openApp(t *testing.T, e *apptest.Env) *app.App {
	t.Helper()
	e.Seed(t, apptest.Span{ID: 99, Start: "08:00", End: "08:30", Bundle: "term", App: "WezTerm", Day: -1})
	a, err := app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func TestDailySnapshot(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm"})
	a := openApp(t, e)
	due, err := a.SnapshotDue()
	if err != nil || !due {
		t.Fatalf("first snapshot due = %v %v", due, err)
	}
	info, removed, err := a.DailySnapshot()
	if err != nil || removed != 0 || info.Kind != snapshot.Daily {
		t.Fatalf("DailySnapshot = %+v %d %v", info, removed, err)
	}
	if filepath.Dir(info.Path) != store.AutoDir(e.DB) {
		t.Fatalf("written to %s", info.Path)
	}
	h, err := a.Health()
	if err != nil || h.BackupMS != e.Clock.UnixMilli() || h.BackupErr != "" || h.IntegrityOK != "ok" || h.Damaged() {
		t.Fatalf("health = %+v %v", h, err)
	}
	if due, _ := a.SnapshotDue(); due {
		t.Fatal("due again right after a snapshot")
	}
	e.Clock = e.Clock.Add(37 * time.Hour)
	if due, _ := a.SnapshotDue(); !due {
		t.Fatal("not due after 37 h")
	}
}

func TestDailySnapshotKeep(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.WriteConfig(t, "[backup]\nkeep = 2\n")
	a := openApp(t, e)
	for i := 0; i < 3; i++ {
		if _, _, err := a.DailySnapshot(); err != nil {
			t.Fatal(err)
		}
		e.Clock = e.Clock.Add(25 * time.Hour)
	}
	l, _ := snapshot.List(store.AutoDir(e.DB))
	if len(l) != 2 {
		t.Fatalf("%d snapshots kept, want 2", len(l))
	}
}

func TestSnapshotOff(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.WriteConfig(t, "[backup]\nkeep = 0\n")
	a := openApp(t, e)
	if due, err := a.SnapshotDue(); err != nil || due {
		t.Fatalf("keep 0 due = %v %v", due, err)
	}
}

func TestNoteDamageOncePerDay(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	a := openApp(t, e)
	for i := 0; i < 3; i++ {
		if err := a.NoteDamage(); err != nil {
			t.Fatal(err)
		}
	}
	ns, err := a.TakeNotices()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, x := range ns {
		if x.Topic == app.TopicIntegrity {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d integrity notices, want 1", n)
	}
}

func TestDamagedHealth(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.State(t, store.KeyIntegrityOK, "*** in database main *** Page 7: btreeInitPage() returns error code 11")
	a := openApp(t, e)
	h, err := a.Health()
	if err != nil || !h.Damaged() {
		t.Fatalf("Damaged = %v %v", h.Damaged(), err)
	}
}

func TestStatusDamaged(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.State(t, store.KeyIntegrityOK, "page 7 broken")
	a := openApp(t, e)
	s, err := a.Status()
	if err != nil || len(s) < 4 || s[:4] != "db! " {
		t.Fatalf("status = %q %v", s, err)
	}
}
