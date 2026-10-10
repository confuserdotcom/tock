package classify

import (
	"sort"

	"tock/internal/span"
)

// Item is one group of spans sharing a Key.
type Item struct {
	Key     string
	App     string
	TotalMS int64
	Spans   int
	// Example is the raw title (else URL) of the most recent span; "" for
	// private spans.
	Example   string
	lastStart int64
}

// Group groups the views for which keep(i) is true by Key, sorted by total
// time descending, then key ascending.
func Group(views []SpanView, keep func(i int) bool) []Item {
	byKey := map[string]*Item{}
	for i := range views {
		if !keep(i) {
			continue
		}
		v := &views[i]
		k := Key(*v)
		it := byKey[k]
		if it == nil {
			it = &Item{Key: k, App: v.App, lastStart: v.Start - 1}
			byKey[k] = it
		}
		it.TotalMS += v.DurMS()
		it.Spans++
		if v.Start > it.lastStart {
			it.lastStart = v.Start
			it.Example = example(v)
		}
	}
	out := make([]Item, 0, len(byKey))
	for _, it := range byKey {
		out = append(out, *it)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].TotalMS != out[b].TotalMS {
			return out[a].TotalMS > out[b].TotalMS
		}
		return out[a].Key < out[b].Key
	})
	return out
}

func example(v *SpanView) string {
	switch {
	case v.Private:
		return ""
	case v.Title != nil && *v.Title != "":
		return *v.Title
	case v.URL != nil:
		return *v.URL
	}
	return ""
}

// Inbox groups spans that have no category, excluding idle spans and ignored
// keys.
func Inbox(views []SpanView, labels []Label, ignored map[string]bool) []Item {
	return Group(views, func(i int) bool {
		return labels[i].Category == "" &&
			views[i].BundleID != span.IdleBundleID &&
			!ignored[Key(views[i])]
	})
}
