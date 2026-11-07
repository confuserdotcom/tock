package snapshot

import "sort"

const (
	hourMS = int64(3600 * 1000)
	// PreKeepMS is how long pre-migration snapshots are kept; the newest
	// one is always kept.
	PreKeepMS = 14 * 24 * hourMS
)

// Plan returns the snapshots to delete: dailies beyond the newest keep
// (none when keep <= 0: daily snapshots are off, old ones stay), and
// pre-migration snapshots older than PreKeepMS except the newest.
func Plan(infos []Info, nowMS int64, keep int) []Info {
	var daily, pre []Info
	for _, i := range infos {
		switch i.Kind {
		case Daily:
			daily = append(daily, i)
		case Pre:
			pre = append(pre, i)
		}
	}
	newestFirst := func(s []Info) {
		sort.SliceStable(s, func(a, b int) bool { return s[a].AtMS > s[b].AtMS })
	}
	newestFirst(daily)
	newestFirst(pre)
	var out []Info
	if keep > 0 && len(daily) > keep {
		out = append(out, daily[keep:]...)
	}
	for k, p := range pre {
		if k > 0 && nowMS-p.AtMS > PreKeepMS {
			out = append(out, p)
		}
	}
	return out
}

// Due reports whether a daily snapshot should be taken now: there is none
// yet, the newest is 36 h old, or it is 24 h old and the user is away.
func Due(newestDailyMS, nowMS int64, away bool) bool {
	if newestDailyMS == 0 {
		return true
	}
	age := nowMS - newestDailyMS
	return age >= 36*hourMS || (age >= 24*hourMS && away)
}
