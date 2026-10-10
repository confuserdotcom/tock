package suggest

import (
	"encoding/json"
	"fmt"

	"tock/internal/classify"
)

// Category is a category name and kind as sent.
type Category struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// PayloadItem is one key as sent: never a raw title, full URL or time of day.
type PayloadItem struct {
	ID      string `json:"id"`
	App     string `json:"app"`
	Host    string `json:"host,omitempty"`
	Title   string `json:"title,omitempty"`
	Minutes int64  `json:"minutes"`
}

// Payload is the whole user message.
type Payload struct {
	Categories []Category    `json:"categories"`
	Projects   []string      `json:"projects"`
	Items      []PayloadItem `json:"items"`
}

// Prepare filters items (sorted largest first, as the inbox returns them),
// skips cached keys, keeps at most BatchMax and builds the batch. It is the
// only way to build a payload, so nothing reaches the bytes unfiltered. An
// empty batch has no Keys and a nil Payload.
func Prepare(items []classify.Item, cached map[string]bool, f *Filter, rs *classify.Ruleset) Batch {
	var picked []classify.Item
	for _, it := range items {
		if len(picked) == BatchMax {
			break
		}
		if !cached[it.Key] && f.Allow(it) {
			picked = append(picked, it)
		}
	}
	b := Batch{Keys: map[string]string{}}
	if len(picked) == 0 {
		return b
	}
	p := Payload{Categories: []Category{}, Projects: []string{}, Items: []PayloadItem{}}
	for _, c := range rs.Categories {
		p.Categories = append(p.Categories, Category{Name: c.Name, Kind: c.Kind})
	}
	for _, pr := range rs.Projects {
		if !pr.Archived {
			p.Projects = append(p.Projects, pr.Name)
		}
	}
	for i, it := range picked {
		id := fmt.Sprintf("k%d", i+1)
		b.Keys[id] = it.Key
		pi := PayloadItem{ID: id, App: it.App, Minutes: (it.TotalMS + 30_000) / 60_000}
		if it.ByURL {
			pi.Host = rest(it)
		} else {
			pi.Title = rest(it)
		}
		p.Items = append(p.Items, pi)
	}
	b.Payload = Marshal(p)
	return b
}

// Marshal is the exact byte form of p: what is sent and what
// `tock suggest --dry-run` prints.
func Marshal(p Payload) []byte {
	out, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		panic(err) // strings and integers only: cannot fail
	}
	return out
}
