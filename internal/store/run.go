package store

import (
	"fmt"

	"tock/internal/stats"
)

// Run states and stop reasons: the stats package reads them back.
const (
	RunRecording = stats.StateRecording
	RunPaused    = stats.StatePaused   // config.toml did not load: nothing recorded
	RunNoAccess  = stats.StateNoAccess // Accessibility is off: apps recorded, window titles not

	RunSignal  = stats.ReasonSignal  // SIGTERM/SIGINT the OS did not say was a session ending: tock, launchctl, kill
	RunSession = stats.ReasonSession // SIGTERM while the OS said the session was ending: logout, restart, shutdown
	RunAsleep  = stats.ReasonAsleep  // trackd's own ticks stopped: the computer slept
	RunState   = stats.ReasonState   // capture paused or resumed; the next row goes on at once
	RunClock   = stats.ReasonClock   // the wall clock went back
	RunAXStale = stats.ReasonAXStale // the Accessibility grant went stale; restarting
	RunUpdate  = stats.ReasonUpdate  // replaced by a new build
	RunError   = stats.ReasonError   // a source or write error; restarting
	RunCrash   = stats.ReasonCrash   // ended without saying why; set by the next start
	RunSchema  = stats.ReasonSchema  // a newer tock raised compat; stays down
)

// RunTickMS is how often trackd writes last_ms; RunSlackMS is how late a
// tick may be. A crashed run ended at most RunTickMS+RunSlackMS after its
// last_ms.
const (
	RunTickMS  int64 = 60_000
	RunSlackMS int64 = 10_000
)

// Run is one trackd_run row. StopMS 0 = still open (running, or crashed
// and not yet closed by the next start).
type Run struct {
	ID                      int64
	StartMS, LastMS, StopMS int64
	Reason, State           string
	BootMS                  int64 // the computer's boot time; 0 = unknown
	PID                     int
	Version                 string
}

// StartRun closes every open row as a crash and inserts r as the open
// row, in one transaction. Only one trackd runs per database (its lock), so
// a row still open when a run starts belongs to a trackd that died. Its
// stop is the later of its last_ms and the 10 s heartbeat, bounded to one
// tick period past last_ms.
func (s *Store) StartRun(r Run) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("store: start run: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // a no-op after Commit
	if _, err := tx.Exec(`UPDATE trackd_run SET reason = ?, stop_ms = MIN(last_ms + ?,
		MAX(last_ms, COALESCE((SELECT CAST(value AS INTEGER) FROM daemon_state WHERE key = ?), 0)))
		WHERE stop_ms IS NULL`, RunCrash, RunTickMS+RunSlackMS, KeyHeartbeat); err != nil {
		return 0, fmt.Errorf("store: start run: close crashed: %w", err)
	}
	if r.LastMS < r.StartMS {
		r.LastMS = r.StartMS
	}
	res, err := tx.Exec(`INSERT INTO trackd_run(start_ms, last_ms, state, boot_ms, pid, version) VALUES(?,?,?,?,?,?)`,
		r.StartMS, r.LastMS, r.State, r.BootMS, r.PID, r.Version)
	if err != nil {
		return 0, fmt.Errorf("store: start run: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: start run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: start run: %w", err)
	}
	return id, nil
}

// TouchRun moves run id's last sign of life to ms (never back).
func (s *Store) TouchRun(id, ms int64) error {
	if _, err := s.db.Exec(`UPDATE trackd_run SET last_ms = MAX(last_ms, ?) WHERE id = ?`, ms, id); err != nil {
		return fmt.Errorf("store: touch run: %w", err)
	}
	return nil
}

// EndRun closes run id at stopMS with reason.
func (s *Store) EndRun(id, stopMS int64, reason string) error {
	if _, err := s.db.Exec(`UPDATE trackd_run SET stop_ms = ?, last_ms = MAX(last_ms, ?), reason = ? WHERE id = ?`,
		stopMS, stopMS, reason, id); err != nil {
		return fmt.Errorf("store: end run: %w", err)
	}
	return nil
}

// Runs returns, by start, the run rows overlapping [fromMS, toMS) plus the
// last row before the range and the first after it: the gaps at the
// range's edges need both their sides.
func (s *Store) Runs(fromMS, toMS int64) ([]Run, error) {
	rows, err := s.db.Query(`SELECT id, start_ms, last_ms, COALESCE(stop_ms, 0), COALESCE(reason, ''), state, boot_ms, pid, version
		FROM trackd_run WHERE id IN (
			SELECT id FROM trackd_run WHERE start_ms < ?2 AND COALESCE(stop_ms, last_ms) >= ?1
			UNION SELECT id FROM (SELECT id FROM trackd_run WHERE COALESCE(stop_ms, last_ms) < ?1 ORDER BY start_ms DESC, id DESC LIMIT 1)
			UNION SELECT id FROM (SELECT id FROM trackd_run WHERE start_ms >= ?2 ORDER BY start_ms, id LIMIT 1))
		ORDER BY start_ms, id`, fromMS, toMS)
	if err != nil {
		return nil, fmt.Errorf("store: runs: %w", err)
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.StartMS, &r.LastMS, &r.StopMS, &r.Reason, &r.State, &r.BootMS, &r.PID, &r.Version); err != nil {
			return nil, fmt.Errorf("store: runs: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: runs: %w", err)
	}
	return out, nil
}
