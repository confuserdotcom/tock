package main

import (
	"path/filepath"
	"testing"
	"time"

	"tock/internal/collect"
	"tock/internal/span"
	outage "tock/internal/stats"
	"tock/internal/store"
)

// withRun gives the rig's pipe a run recorder on its store, with a fixed
// boot time.
func (r *pauseRig) withRun(boot int64) *pauseRig {
	r.p.rec = &runRec{st: r.st, boot: func() int64 { return boot }, version: "test", pid: 1}
	return r
}

// gaps reads every run row back and computes the gaps in [from, to) at now.
func (r *pauseRig) gaps(from, to, now int64) []outage.Gap {
	r.t.Helper()
	return readGaps(r.t, r.st, from, to, now)
}

func readGaps(t *testing.T, st *store.Store, from, to, now int64) []outage.Gap {
	t.Helper()
	rs, err := st.Runs(from, to)
	if err != nil {
		t.Fatal(err)
	}
	ss := make([]outage.Stretch, len(rs))
	for i, x := range rs {
		ss[i] = outage.Stretch{Start: x.StartMS, Last: x.LastMS, Stop: x.StopMS, Reason: x.Reason, State: x.State, Boot: x.BootMS}
	}
	return outage.Gaps(ss, from, to, now)
}

func (r *pauseRig) runs() []store.Run {
	r.t.Helper()
	rs, err := r.st.Runs(0, 1<<62)
	if err != nil {
		r.t.Fatal(err)
	}
	return rs
}

// ticks drives the pipe's 10 s tick from a to b inclusive.
func (r *pauseRig) ticks(a, b int64) {
	r.t.Helper()
	for ts := a; ts <= b; ts += 10_000 {
		if err := r.p.tickAt(ts); err != nil {
			r.t.Fatal(err)
		}
	}
}

// With nothing in front (no window, no samples) trackd still runs: its row
// advances each minute and there is no gap.
func TestRunNoWindowIsNoOutage(t *testing.T) {
	r := newPauseRig(t, goodConfig).withRun(1)
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	r.ticks(pauseT0+10_000, pauseT0+30*60_000)
	rs := r.runs()
	if len(rs) != 1 || rs[0].StopMS != 0 || rs[0].State != store.RunRecording {
		t.Fatalf("runs = %+v, want one open recording row", rs)
	}
	if rs[0].LastMS != pauseT0+30*60_000 {
		t.Fatalf("last_ms = %d, want %d", rs[0].LastMS, pauseT0+30*60_000)
	}
	if g := r.gaps(pauseT0, pauseT0+31*60_000, pauseT0+30*60_000+5_000); len(g) != 0 {
		t.Fatalf("gaps = %+v, want none", g)
	}
	if r.spans() != 0 {
		t.Fatal("ticks without samples wrote spans")
	}
}

// last_ms is written once a minute, not on every 10 s tick.
func TestRunTouchesOncePerMinute(t *testing.T) {
	r := newPauseRig(t, goodConfig).withRun(1)
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	r.ticks(pauseT0+10_000, pauseT0+50_000)
	if got := r.runs()[0].LastMS; got != pauseT0 {
		t.Fatalf("last_ms after 50 s = %d, want the start %d", got, pauseT0)
	}
	r.ticks(pauseT0+60_000, pauseT0+60_000)
	if got := r.runs()[0].LastMS; got != pauseT0+60_000 {
		t.Fatalf("last_ms after 60 s = %d", got)
	}
}

// AC1: a trackd killed without a word, started again 5 min later. The
// gap runs from the kill to the new start, within 10 s, and is a crash.
func TestRunCrashThenRestart(t *testing.T) {
	r := newPauseRig(t, goodConfig).withRun(1)
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	// a window is open, so the 10 s heartbeat runs alongside the minute tick
	ts := pauseT0
	for ; ts <= pauseT0+95_000; ts += 1000 {
		r.sample(ts, "editor", "work", "")
		if (ts-pauseT0)%10_000 == 0 {
			if err := r.p.tickAt(ts); err != nil {
				t.Fatal(err)
			}
		}
	}
	kill := pauseT0 + 95_000 // SIGKILL: no end, no flush
	restart := kill + 5*60_000

	r2 := &pauseRig{t: t, cfgDir: r.cfgDir, path: r.path, st: r.st}
	r2.p = pipe{m: span.New(span.DefaultConfig()), st: r.st, now: func() int64 { return restart }, hide: newHider(r.cfgDir)}
	r2.withRun(1)
	if _, err := r.st.Recover(); err != nil {
		t.Fatal(err)
	}
	if err := r2.p.start(); err != nil {
		t.Fatal(err)
	}
	g := r2.gaps(pauseT0, restart+60_000, restart+10_000)
	if len(g) != 1 || g[0].Kind != outage.GapCrash || g[0].To != restart {
		t.Fatalf("gaps = %+v, want one crash ending at the restart", g)
	}
	if d := kill - g[0].From; d < 0 || d > 10_000 {
		t.Fatalf("crash gap starts %d ms before the kill, want 0..10000", d)
	}
	if o := outage.Outages(g); len(o) != 1 {
		t.Fatalf("outages = %+v", o)
	}
}

// A clean stop, then a start an hour later: stopped, not a crash.
func TestRunStopThenStart(t *testing.T) {
	r := newPauseRig(t, goodConfig).withRun(1)
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	r.ticks(pauseT0+10_000, pauseT0+120_000)
	if err := r.p.rec.end(pauseT0+125_000, store.RunSignal); err != nil {
		t.Fatal(err)
	}
	later := pauseT0 + 125_000 + 60*60_000
	r.p.now = func() int64 { return later }
	r.p.rec = &runRec{st: r.st, boot: func() int64 { return 1 }}
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	g := r.gaps(pauseT0, later+60_000, later+1000)
	if len(g) != 1 || g[0] != (outage.Gap{From: pauseT0 + 125_000, To: later, Kind: outage.GapStopped}) {
		t.Fatalf("gaps = %+v", g)
	}
	// stopped before a shutdown: the next start is in a new boot
	r.p.rec.end(later+10_000, store.RunSignal) //nolint:errcheck
	next := later + 3*60*60_000
	r.p.now = func() int64 { return next }
	r.p.rec = &runRec{st: r.st, boot: func() int64 { return next - 60_000 }}
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	g = r.gaps(later, next+60_000, next+1000)
	if len(g) != 1 || g[0].Kind != outage.GapOff {
		t.Fatalf("gaps across a reboot = %+v, want off", g)
	}
	if len(outage.Outages(g)) != 0 {
		t.Fatal("a computer that was off is not an outage")
	}
}

// The computer sleeps: trackd's ticks stop and resume an hour later. The
// gap is sleep, not an outage, and the row starts again at wake.
func TestRunSleepIsNotOutage(t *testing.T) {
	r := newPauseRig(t, goodConfig).withRun(1)
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	r.ticks(pauseT0+10_000, pauseT0+300_000)
	wake := pauseT0 + 300_000 + 60*60_000
	r.ticks(wake, wake+120_000)
	rs := r.runs()
	if len(rs) != 2 || rs[0].Reason != store.RunAsleep || rs[0].StopMS != pauseT0+300_000 || rs[1].StartMS != wake {
		t.Fatalf("runs = %+v", rs)
	}
	g := r.gaps(pauseT0, wake+200_000, wake+120_000)
	if len(g) != 1 || g[0] != (outage.Gap{From: pauseT0 + 300_000, To: wake, Kind: outage.GapAsleep}) {
		t.Fatalf("gaps = %+v", g)
	}
	if len(outage.Outages(g)) != 0 {
		t.Fatal("sleep counted as an outage")
	}
}

// The wall clock goes back: the row ends and a new one starts; no gap.
func TestRunClockBack(t *testing.T) {
	r := newPauseRig(t, goodConfig).withRun(1)
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	r.ticks(pauseT0+10_000, pauseT0+120_000)
	r.ticks(pauseT0+30_000, pauseT0+90_000) // an hour's NTP step would look the same
	rs := r.runs()
	if len(rs) != 2 || rs[0].Reason != store.RunClock {
		t.Fatalf("runs = %+v", rs)
	}
	if g := r.gaps(pauseT0, pauseT0+200_000, pauseT0+95_000); len(g) != 0 {
		t.Fatalf("gaps = %+v, want none", g)
	}
}

// T23 regression: config.toml breaks mid-run and is fixed later. The
// paused time is its own row and shows as "config"; nothing recorded in it.
func TestRunConfigPauseIsOutage(t *testing.T) {
	r := newPauseRig(t, goodConfig).withRun(1)
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	r.ticks(pauseT0+10_000, pauseT0+50_000)
	r.writeConfig(pausedConfigs["newer keys"])
	broke := pauseT0 + hideEvery
	r.ticks(broke, broke+20*60_000)
	r.sample(broke+5*60_000, "diary", "secret diary", "")
	r.writeConfig(goodConfig)
	fixed := broke + 20*60_000 + hidePausedEvery
	r.ticks(fixed, fixed+120_000)
	rs := r.runs()
	if len(rs) != 3 || rs[1].State != store.RunPaused || rs[1].StartMS != broke || rs[1].StopMS != fixed || rs[2].State != store.RunRecording {
		t.Fatalf("runs = %+v", rs)
	}
	g := r.gaps(pauseT0, fixed+200_000, fixed+120_000)
	if len(g) != 1 || g[0] != (outage.Gap{From: broke, To: fixed, Kind: outage.GapConfig}) {
		t.Fatalf("gaps = %+v", g)
	}
	if r.leaked("secret") != 0 {
		t.Fatal("hidden title stored while paused")
	}
}

// T23 regression: trackd starts with a broken config.toml. Its first row
// is paused from the start.
func TestRunStartsPaused(t *testing.T) {
	for name, cfg := range pausedConfigs {
		t.Run(name, func(t *testing.T) {
			r := newPauseRig(t, cfg).withRun(1)
			if err := r.p.start(); err != nil {
				t.Fatal(err)
			}
			r.ticks(pauseT0+10_000, pauseT0+5*60_000)
			rs := r.runs()
			if len(rs) != 1 || rs[0].State != store.RunPaused || rs[0].StartMS != pauseT0 {
				t.Fatalf("runs = %+v", rs)
			}
			g := r.gaps(pauseT0, pauseT0+10*60_000, pauseT0+5*60_000+1000)
			if len(g) != 1 || g[0].Kind != outage.GapConfig || g[0].From != pauseT0 {
				t.Fatalf("gaps = %+v", g)
			}
		})
	}
}

// The whole daemon: a clean exit ends the row with a reason, a stale
// Accessibility grant with ax_stale, and the rows persist across restarts.
func TestDaemonRecordsRuns(t *testing.T) {
	dir := t.TempDir()
	if r := runFakeDaemon(t, dir, samplesSource()); r.rc != 0 {
		t.Fatalf("rc = %d\n%s", r.rc, r.log)
	}
	if r := runFakeDaemon(t, dir, &staleSource{ch: make(chan span.Input)}); r.rc != 1 {
		t.Fatalf("rc = %d\n%s", r.rc, r.log)
	}
	st, err := store.Open(filepath.Join(dir, "tock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rs, err := st.Runs(0, time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("runs = %+v, want 2", rs)
	}
	if rs[0].Reason != store.RunSignal || rs[0].StopMS == 0 || rs[1].Reason != store.RunAXStale {
		t.Fatalf("runs = %+v, want signal then ax_stale", rs)
	}
	if rs[0].Version != version || rs[0].PID == 0 {
		t.Fatalf("run identity = %+v", rs[0])
	}
}

var _ collect.Source = (*staleSource)(nil)

// A locked screen closes the span and nothing is recorded until unlock,
// but trackd runs throughout: away, not an outage.
func TestRunLockedScreenIsNoOutage(t *testing.T) {
	r := newPauseRig(t, goodConfig).withRun(1)
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	r.sample(pauseT0, "editor", "work", "")
	lock := pauseT0 + 60_000
	if err := r.p.feed(span.Input{Event: &span.Event{TS: lock, Kind: span.EventLock}}); err != nil {
		t.Fatal(err)
	}
	r.ticks(lock+10_000, lock+40*60_000)
	unlock := lock + 40*60_000 + 5_000
	if err := r.p.feed(span.Input{Event: &span.Event{TS: unlock, Kind: span.EventUnlock}}); err != nil {
		t.Fatal(err)
	}
	r.sample(unlock+1000, "editor", "work", "")
	r.ticks(unlock+10_000, unlock+60_000)
	if g := r.gaps(pauseT0, unlock+120_000, unlock+60_000); len(g) != 0 {
		t.Fatalf("gaps = %+v, want none", g)
	}
	if rs := r.runs(); len(rs) != 1 {
		t.Fatalf("runs = %+v, want one", rs)
	}
}
