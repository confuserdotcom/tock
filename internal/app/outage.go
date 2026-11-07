package app

import (
	"tock/internal/stats"
	"tock/internal/store"
)

// Outages is the time in [fromMS, toMS), up to now, that trackd should have
// recorded and did not, and why (stats.Outages over the trackd_run rows).
// Sleep and a computer that was off are not outages. Outages are never
// spans: they count as neither tracked nor idle time.
func (a *App) Outages(fromMS, toMS int64) ([]stats.Gap, error) {
	return ReadOutages(a.st, fromMS, toMS, a.nowMS())
}

// ReadOutages is Outages for callers with a store only.
func ReadOutages(st *store.Store, fromMS, toMS, nowMS int64) ([]stats.Gap, error) {
	rs, err := st.Runs(fromMS, toMS)
	if err != nil {
		return nil, err
	}
	ss := make([]stats.Stretch, len(rs))
	for i, r := range rs {
		ss[i] = stats.Stretch{Start: r.StartMS, Last: r.LastMS, Stop: r.StopMS, Reason: r.Reason, State: r.State, Boot: r.BootMS}
	}
	return stats.Outages(stats.Gaps(ss, fromMS, toMS, nowMS)), nil
}
