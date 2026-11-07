package main

import (
	"log/slog"

	"tock/internal/store"
)

// runGapMS is how long the 10 s ticks may stop before trackd counts the
// computer as having slept (or itself as suspended): a gap that long ends
// the row and the next tick starts a new one.
const runGapMS int64 = 90_000

// runClockBackMS is how far a tick may come before the last one before the
// wall clock counts as having gone back.
const runClockBackMS int64 = 5_000

// runRec keeps trackd's rows in trackd_run: one row per stretch trackd ran
// without a break in one capture state. The row's last_ms moves once a
// minute on trackd's own tick, whether or not anything is recorded, so the
// time between rows (and in paused rows) is time nothing was recorded.
// A nil runRec records nothing.
type runRec struct {
	st      *store.Store
	log     *slog.Logger // may be nil
	boot    func() int64
	version string
	pid     int

	id    int64 // the open row; 0 = none
	state string
	alive int64 // the last tick
	wrote int64 // the last last_ms written
}

// begin opens a row at ts in state. Rows a dead trackd left open are
// closed as crashes first (store.StartRun).
func (r *runRec) begin(ts int64, state string) error {
	if r == nil {
		return nil
	}
	var boot int64
	if r.boot != nil {
		boot = r.boot()
	}
	id, err := r.st.StartRun(store.Run{StartMS: ts, State: state, BootMS: boot, PID: r.pid, Version: r.version})
	if err != nil {
		return err
	}
	r.id, r.state, r.alive, r.wrote = id, state, ts, ts
	return nil
}

// end closes the open row at ts with reason.
func (r *runRec) end(ts int64, reason string) error {
	if r == nil || r.id == 0 {
		return nil
	}
	err := r.st.EndRun(r.id, max(ts, r.alive), reason)
	r.id = 0
	return err
}

// tick is trackd's 10 s tick at ts. After a gap (sleep) or a step back
// of the clock it ends the row and starts another; otherwise it writes
// last_ms once a minute.
func (r *runRec) tick(ts int64) error {
	if r == nil || r.id == 0 {
		return nil
	}
	switch {
	case ts-r.alive > runGapMS:
		return r.restart(r.alive, ts, store.RunAsleep)
	case ts < r.alive-runClockBackMS:
		return r.restart(r.alive, ts, store.RunClock)
	}
	r.alive = max(r.alive, ts)
	if ts-r.wrote < store.RunTickMS {
		return nil
	}
	r.wrote = ts
	return r.st.TouchRun(r.id, ts)
}

// restart ends the row at stop for reason and opens the next at ts.
func (r *runRec) restart(stop, ts int64, reason string) error {
	if r.log != nil {
		r.log.Info("trackd run row restarted", "reason", reason, "gap_ms", ts-stop)
	}
	if err := r.end(stop, reason); err != nil {
		return err
	}
	return r.begin(ts, r.state)
}

// setState ends the row and starts one in state at ts, when the state
// changes. A gap before ts (the computer woke and the first thing seen
// changed the state) is ended as sleep first.
func (r *runRec) setState(ts int64, state string) error {
	if r == nil || r.id == 0 || state == r.state {
		return nil
	}
	if ts-r.alive > runGapMS {
		if err := r.restart(r.alive, ts, store.RunAsleep); err != nil {
			return err
		}
	}
	at := max(ts, r.alive)
	if err := r.end(at, store.RunState); err != nil {
		return err
	}
	return r.begin(at, state)
}
