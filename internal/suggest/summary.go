package suggest

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/span"
)

// SummaryItemsMax is the most items sent per block.
const SummaryItemsMax = 8

// SummaryItem is one app with a host or cleaned title, as sent.
type SummaryItem struct {
	App     string `json:"app"`
	Host    string `json:"host,omitempty"`
	Title   string `json:"title,omitempty"`
	Minutes int64  `json:"minutes"`
}

// SummaryBlock is one work block as sent: clock times, never dates.
type SummaryBlock struct {
	ID       string        `json:"id"`
	From     string        `json:"from"`
	To       string        `json:"to"`
	Minutes  int64         `json:"minutes"`
	Label    string        `json:"label"`
	Items    []SummaryItem `json:"items"`
	Withheld int64         `json:"withheld_minutes,omitempty"` // time in items the filter kept back
}

// SummaryPayload is the whole user message of tock summary.
type SummaryPayload struct {
	Day    string         `json:"day"` // weekday only
	Blocks []SummaryBlock `json:"blocks"`
}

// SummaryBatch is one summary request: the bytes and each block id's start.
type SummaryBatch struct {
	Payload []byte
	Starts  map[string]int64
}

// PrepareSummary builds the summary payload of a day's blocks from views
// and labels (start order). Items go through f like inbox keys: private,
// denied and short ones are withheld and only counted. A day without blocks
// gives an empty batch.
func PrepareSummary(day time.Time, blocks []report.Block, views []classify.SpanView, labels []classify.Label, f *Filter) SummaryBatch {
	b := SummaryBatch{Starts: map[string]int64{}}
	if len(blocks) == 0 {
		return b
	}
	loc := day.Location()
	clock := func(ms int64) string { return time.UnixMilli(ms).In(loc).Format("15:04") }
	p := SummaryPayload{Day: day.Weekday().String(), Blocks: []SummaryBlock{}}
	for i, bl := range blocks {
		id := fmt.Sprintf("b%d", i+1)
		b.Starts[id] = bl.Start
		label := bl.Name
		if bl.Project != "" && bl.Category != "" {
			label += " (" + bl.Category + ")"
		}
		if bl.Kind != "" {
			label += ", " + bl.Kind
		}
		sb := SummaryBlock{ID: id, From: clock(bl.Start), To: clock(bl.End), Minutes: mins(bl.ActiveMS), Label: label, Items: []SummaryItem{}}
		inBlock := func(v classify.SpanView) bool {
			return v.Start >= bl.Start && v.End <= bl.End && v.BundleID != span.IdleBundleID
		}
		// Another machine's time is labelled and summarized there.
		for _, v := range views {
			if inBlock(v) && classify.IsRemote(v) {
				sb.Withheld += mins(v.DurMS())
			}
		}
		items := classify.Group(views, func(i int) bool { return inBlock(views[i]) && !classify.IsRemote(views[i]) })
		for _, it := range items {
			if len(sb.Items) == SummaryItemsMax || !f.Allow(it) {
				sb.Withheld += mins(it.TotalMS)
				continue
			}
			si := SummaryItem{App: it.App, Minutes: mins(it.TotalMS)}
			if r := rest(it); it.ByURL {
				si.Host = r
			} else {
				si.Title = r
			}
			sb.Items = append(sb.Items, si)
		}
		p.Blocks = append(p.Blocks, sb)
	}
	out, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		panic(err) // strings and integers only
	}
	b.Payload = out
	return b
}

func mins(ms int64) int64 { return (ms + 30_000) / 60_000 }

// Summary is Claude's answer: the day, and a line per block start.
type Summary struct {
	Day    string
	Blocks map[int64]string
}

// SummaryMaxRunes bounds every text kept from the answer.
const SummaryMaxRunes = 600

// ParseSummary reads the summarize tool input. Unknown ids are dropped and
// texts are cleaned and shortened.
func ParseSummary(raw json.RawMessage, starts map[string]int64) (Summary, error) {
	var in struct {
		Day    string `json:"day"`
		Blocks []struct {
			ID      string `json:"id"`
			Summary string `json:"summary"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return Summary{}, fmt.Errorf("summary: bad answer: %w", err)
	}
	s := Summary{Day: cleanText(in.Day), Blocks: map[int64]string{}}
	if s.Day == "" {
		return Summary{}, fmt.Errorf("summary: empty answer")
	}
	for _, b := range in.Blocks {
		if start, ok := starts[b.ID]; ok {
			if t := cleanText(b.Summary); t != "" {
				s.Blocks[start] = t
			}
		}
	}
	return s, nil
}

// cleanText keeps printable text on one line, at most SummaryMaxRunes.
func cleanText(s string) string {
	s = report.Safe(strings.Join(strings.Fields(s), " "))
	if r := []rune(s); len(r) > SummaryMaxRunes {
		s = string(r[:SummaryMaxRunes-1]) + "…"
	}
	return s
}
