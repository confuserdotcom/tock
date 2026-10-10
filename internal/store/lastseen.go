package store

// LastSeen returns the latest sign of a running trackd: the larger of the
// last heartbeat and the newest span start, in unix ms (0 when trackd never
// wrote anything). The heartbeat advances every 10 s while a span is open;
// the span start covers the seconds after unlock or wake before the first
// heartbeat.
func (s *Store) LastSeen() (int64, error) {
	var ms int64
	err := s.db.QueryRow(`SELECT max(
		coalesce((SELECT CAST(value AS INTEGER) FROM daemon_state WHERE key = ?), 0),
		coalesce((SELECT max(start) FROM span), 0))`, KeyHeartbeat).Scan(&ms)
	return ms, err
}
