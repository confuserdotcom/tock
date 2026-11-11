package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"tock/internal/classify"
	"tock/internal/report"
)

// A timeline row on an issued invoice names it and underlines its
// duration, as the dashboard does; other rows don't.
func TestTimelineRowBilled(t *testing.T) {
	r := report.Row{Start: 0, End: 30 * 60_000, App: "WezTerm", IDs: []int64{7}, Label: classify.Label{Category: "Code", Kind: "deep"}}
	tt := todayTab{billed: map[int64]string{7: "2026-0001"}, sugg: map[string]classify.Want{}}
	got := tt.timelineRow(0, r, 120, false, time.UTC)
	if !strings.Contains(ansi.Strip(got), "on 2026-0001") || !strings.Contains(got, "\x1b[") {
		t.Fatalf("billed row: %q", got)
	}
	r.IDs = []int64{8}
	if got := ansi.Strip(tt.timelineRow(0, r, 120, false, time.UTC)); strings.Contains(got, "2026-0001") {
		t.Fatalf("unbilled row: %q", got)
	}
}
