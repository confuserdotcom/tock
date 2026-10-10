package focus

import (
	"time"

	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/span"
)

const minute = int64(60_000)

var (
	zone = time.FixedZone("T", 2*3600)
	day0 = time.Date(2026, 10, 10, 0, 0, 0, 0, zone)
)

// at is h:m:s on day0 in unix ms.
func at(h, m, s int) int64 {
	return day0.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second).UnixMilli()
}

// vw is one synthetic view. kind is a category kind, "" (unlabelled) or
// "idle"; bundle defaults to "app".
type vw struct {
	from, to int64
	kind     string
	bundle   string
	url      string
}

func world(vs ...vw) ([]classify.SpanView, []classify.Label) {
	views := make([]classify.SpanView, len(vs))
	labels := make([]classify.Label, len(vs))
	for i, x := range vs {
		b := x.bundle
		if b == "" {
			b = "app"
		}
		if x.kind == "idle" {
			b = span.IdleBundleID
		}
		views[i] = classify.SpanView{ID: int64(i + 1), Start: x.from, End: x.to, BundleID: b, App: b}
		if x.url != "" {
			u := x.url
			views[i].URL = &u
		}
		if x.kind != "idle" {
			labels[i].Kind = x.kind
		}
	}
	return views, labels
}

// input is a tick at now over vs with the default config.
func input(now int64, vs ...vw) Input {
	views, labels := world(vs...)
	return Input{Now: now, Day: day0, Views: views, Labels: labels, Cfg: config.DefaultFocus()}
}

// only returns the events of kind k.
func only(evs []Event, k EventKind) []Event {
	var out []Event
	for _, e := range evs {
		if e.Kind == k {
			out = append(out, e)
		}
	}
	return out
}
