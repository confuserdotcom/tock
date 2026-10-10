package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/span"
)

// Work block tuning.
const (
	// BlockBreakMS is idle time, or a gap, that ends a block.
	BlockBreakMS = 10 * 60_000
	// BlockSwitchMS is how long other work has to last to start a block
	// of its own; shorter stretches stay inside the block around them.
	BlockSwitchMS = 10 * 60_000
	// BlockMinMS is the shortest block listed.
	BlockMinMS = 15 * 60_000
)

// Block is a stretch of work on one thing: a project, else a category.
type Block struct {
	Start, End        int64
	Name              string // the project, else the category, else Unlabelled
	Project, Category string // the category with most of the block's time
	Kind              string // that category's kind
	ActiveMS          int64
	DeepMS            int64
	DistractMS        int64
	Top               []NameMS // the apps and sites with most of its time, at most 3
	Summary           string   // Claude's line about it, filled by the caller when saved
}

// blockKey is what a view counts towards: its project, else its category.
func blockKey(l classify.Label) string {
	if l.Project != "" {
		return "p:" + l.Project
	}
	return "c:" + l.Category
}

type blockRun struct {
	key        string
	start, end int64
	idx        []int // view indexes
	ms         int64
}

// Blocks groups the non-idle views into work blocks: a block ends at a
// break of BlockBreakMS, or when other work lasts BlockSwitchMS; shorter
// detours stay inside it. Blocks under BlockMinMS of activity are left
// out. views must be in start order; labels[i] labels views[i].
func Blocks(views []classify.SpanView, labels []classify.Label) []Block {
	// Runs of one key, cut at breaks.
	var runs []blockRun
	var lastEnd int64
	breakBefore := map[int]bool{} // run index → a break precedes it
	for i, v := range views {
		if v.BundleID == span.IdleBundleID {
			continue
		}
		k := blockKey(labels[i])
		n := len(runs)
		brk := n > 0 && v.Start-lastEnd >= BlockBreakMS
		if n == 0 || brk || runs[n-1].key != k {
			runs = append(runs, blockRun{key: k, start: v.Start})
			breakBefore[len(runs)-1] = n == 0 || brk
			n++
		}
		r := &runs[n-1]
		r.end = max(r.end, v.End)
		r.idx = append(r.idx, i)
		r.ms += v.DurMS()
		lastEnd = max(lastEnd, v.End)
	}
	// Merge: a short run joins the block before it; a long one starts a
	// block unless it has the block's key.
	var groups []blockRun
	for i, r := range runs {
		n := len(groups)
		switch {
		case n == 0 || breakBefore[i]:
			groups = append(groups, r)
		case r.ms < BlockSwitchMS || r.key == groups[n-1].key:
			g := &groups[n-1]
			g.end, g.idx, g.ms = max(g.end, r.end), append(g.idx, r.idx...), g.ms+r.ms
		default:
			groups = append(groups, r)
		}
	}
	// A block is named after the key with most of its time (buildBlock), not
	// the run that opened it.
	var out []Block
	for _, g := range groups {
		if g.ms < BlockMinMS {
			continue
		}
		out = append(out, buildBlock(g, views, labels))
	}
	return out
}

func buildBlock(g blockRun, views []classify.SpanView, labels []classify.Label) Block {
	b := Block{Start: g.start, End: g.end}
	keys := map[string]int64{}
	cats := map[string]int64{}
	kinds := map[string]string{}
	tops := map[string]int64{}
	for _, i := range g.idx {
		v, l := views[i], labels[i]
		d := v.DurMS()
		b.ActiveMS += d
		keys[blockKey(l)] += d
		cats[l.Category] += d
		kinds[l.Category] = l.Kind
		switch l.Kind {
		case classify.KindDeep:
			b.DeepMS += d
		case classify.KindDistract:
			b.DistractMS += d
		}
		name := v.App
		if !v.Private && v.URL != nil && *v.URL != "" {
			h, _, _ := strings.Cut(strings.TrimPrefix(*v.URL, "www."), "/")
			name = strings.ToLower(h)
		}
		tops[name] += d
	}
	key := mostly(keys)
	b.Category = mostly(cats)
	b.Kind = kinds[b.Category]
	switch {
	case strings.HasPrefix(key, "p:"):
		b.Project = key[2:]
		b.Name = b.Project
	case b.Category != "":
		b.Name = b.Category
	default:
		b.Name = "Unlabelled"
	}
	for n, d := range tops {
		b.Top = append(b.Top, NameMS{n, d})
	}
	sort.Slice(b.Top, func(i, j int) bool {
		if b.Top[i].MS != b.Top[j].MS {
			return b.Top[i].MS > b.Top[j].MS
		}
		return b.Top[i].Name < b.Top[j].Name
	})
	if len(b.Top) > 3 {
		b.Top = b.Top[:3]
	}
	return b
}

// BlockLine is one block as text: "09:00–10:40  1h40  tock (Code) · deep 92% · WezTerm, github.com".
func BlockLine(b Block, loc *time.Location) string {
	clock := func(ms int64) string { return time.UnixMilli(ms).In(loc).Format("15:04") }
	name := Safe(b.Name)
	if b.Project != "" && b.Category != "" {
		name += " (" + Safe(b.Category) + ")"
	}
	line := fmt.Sprintf("%s–%s  %5s  %s", clock(b.Start), clock(b.End), Dur(b.ActiveMS), name)
	if b.ActiveMS > 0 && b.DeepMS > 0 {
		line += fmt.Sprintf(" · deep %d%%", pct(b.DeepMS, b.ActiveMS))
	}
	if b.DistractMS >= 60_000 {
		line += " · distracted " + Dur(b.DistractMS)
	}
	var tops []string
	for _, t := range b.Top {
		tops = append(tops, Safe(t.Name))
	}
	if len(tops) > 0 {
		line += " · " + strings.Join(tops, ", ")
	}
	return line
}
