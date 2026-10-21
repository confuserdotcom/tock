package report

import "sort"

// Share is a project's part of the tracked time; Name "" is time with no
// project.
type Share struct {
	Name string
	MS   int64
	Pct  int
}

// Shares turns time per project into percentages of their total, rounded so
// they sum to 100 (largest remainder). Projects come by time, then name,
// and the time with no project ("") last. Nil when no project has time.
func Shares(ms map[string]int64) []Share {
	var total int64
	out := make([]Share, 0, len(ms))
	named := false
	for n, v := range ms {
		if v <= 0 {
			continue
		}
		total += v
		named = named || n != ""
		out = append(out, Share{Name: n, MS: v})
	}
	if !named {
		return nil
	}
	sort.Slice(out, func(a, b int) bool {
		if (out[a].Name == "") != (out[b].Name == "") {
			return out[b].Name == ""
		}
		if out[a].MS != out[b].MS {
			return out[a].MS > out[b].MS
		}
		return out[a].Name < out[b].Name
	})
	rem := make([]int64, len(out))
	left := 100
	for i := range out {
		out[i].Pct = int(out[i].MS * 100 / total)
		rem[i] = out[i].MS * 100 % total
		left -= out[i].Pct
	}
	idx := make([]int, len(out))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return rem[idx[a]] > rem[idx[b]] })
	for _, i := range idx[:left] {
		out[i].Pct++
	}
	return out
}
