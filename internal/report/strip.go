package report

import "sort"

// Strip splits a day's time into n cells for the menu bar's colour strip:
// each cell is a kind ("" = unlabelled), in Kinds order, with cell counts by
// largest remainder so they sum to n. Kinds not in Kinds are left out. Nil
// when nothing is tracked.
func Strip(byKind map[string]int64, n int) []string {
	var total int64
	for _, k := range Kinds {
		total += max(byKind[k], 0)
	}
	if total <= 0 || n <= 0 {
		return nil
	}
	type rem struct {
		i int
		r int64
	}
	cells := make([]int, len(Kinds))
	rems := make([]rem, 0, len(Kinds))
	used := 0
	for i, k := range Kinds {
		v := max(byKind[k], 0) * int64(n)
		cells[i] = int(v / total)
		used += cells[i]
		rems = append(rems, rem{i, v % total})
	}
	sort.SliceStable(rems, func(a, b int) bool { return rems[a].r > rems[b].r })
	for j := 0; used < n; j++ {
		cells[rems[j].i]++
		used++
	}
	out := make([]string, 0, n)
	for i, k := range Kinds {
		for range cells[i] {
			out = append(out, k)
		}
	}
	return out
}
