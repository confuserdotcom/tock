package report

import (
	"reflect"
	"testing"

	"tock/internal/stats"
)

func TestGapWhy(t *testing.T) {
	if got := GapWhy(stats.Gap{Kind: stats.GapCrash}); got != "trackd stopped unexpectedly" {
		t.Errorf("crash: %q", got)
	}
	got := GapWhy(stats.Gap{Kind: stats.GapCrash, EndUnknown: true})
	if got != "trackd stopped unexpectedly (until the restart; the computer may have been off for part of it)" {
		t.Errorf("crash, end unknown: %q", got)
	}
	if got := GapWhy(stats.Gap{Kind: stats.GapNoAccess}); got != "Accessibility is off: apps only, no window titles" {
		t.Errorf("no access: %q", got)
	}
}

func TestSplitPartial(t *testing.T) {
	a := stats.Gap{From: 1, To: 2, Kind: stats.GapCrash}
	b := stats.Gap{From: 3, To: 4, Kind: stats.GapNoAccess}
	c := stats.Gap{From: 5, To: 6, Kind: stats.GapConfig}
	none, partial := SplitPartial([]stats.Gap{a, b, c})
	if !reflect.DeepEqual(none, []stats.Gap{a, c}) || !reflect.DeepEqual(partial, []stats.Gap{b}) {
		t.Fatalf("none %+v partial %+v", none, partial)
	}
}
