package span

import (
	"strconv"
	"testing"
)

// BenchmarkMergerWorkday feeds one 8-hour workday of 2 s polls, the rate
// trackd polls at: a window switch every 90 s across five apps, a URL on the
// browser, and a 20-minute idle stretch after lunch. One op is one day.
func BenchmarkMergerWorkday(b *testing.B) {
	const day = 8 * 3600 * 1000
	apps := []string{"term", "browser", "editor", "mail", "chat"}
	ins := make([]Input, 0, day/2000)
	activity := t0
	for ts := t0; ts < t0+day; ts += 2000 {
		n := (ts - t0) / 90_000
		bundle := apps[n%int64(len(apps))]
		idle := 0.0
		// 4h00-4h20 in: nobody touches the machine.
		if ts-t0 >= 4*3600*1000 && ts-t0 < 4*3600*1000+20*60*1000 {
			idle = float64(ts-activity) / 1000
		} else {
			activity = ts
		}
		title := bundle + " " + strconv.FormatInt(n%7, 10)
		s := &Sample{TS: ts, BundleID: bundle, App: bundle, IdleS: idle, Title: &title}
		if bundle == "browser" {
			u := "example.com/page/" + strconv.FormatInt(n%11, 10)
			s.URL = &u
		}
		ins = append(ins, Input{Sample: s})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := New(DefaultConfig())
		for _, in := range ins {
			m.Feed(in)
		}
		m.Flush()
	}
	b.ReportMetric(float64(len(ins)), "samples/op")
}
