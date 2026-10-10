package report

import (
	"fmt"
	"strings"
)

// ScoreLine renders the plain score summary shared by the TUI and tock
// today, for example
// "score 72 · focus 2h10 / 4h00 · 3 blocks, longest 48m · 14 switches".
// With no active time it is "score — · no activity".
func ScoreLine(d Day) string {
	st, sc := d.Stats, d.Score
	if st.ActiveMS <= 0 {
		return "score — · no activity"
	}
	blocks := "no blocks"
	if n := len(st.Blocks); n > 0 {
		blocks = fmt.Sprintf("%d %s, longest %s", n, plural(n, "block", "blocks"), Dur(st.LongestMS))
	}
	return strings.Join([]string{
		fmt.Sprintf("score %d", sc.Value),
		fmt.Sprintf("focus %s / %s", Dur(st.FocusMS), Dur(sc.Target.MS)),
		blocks,
		fmt.Sprintf("%d %s", st.Switches, plural(st.Switches, "switch", "switches")),
	}, " · ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
