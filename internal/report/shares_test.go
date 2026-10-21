package report

import (
	"reflect"
	"testing"
)

func TestShares(t *testing.T) {
	got := Shares(map[string]int64{"tock": 3, "thesis": 3, "": 3, "zero": 0})
	want := []Share{{"thesis", 3, 34}, {"tock", 3, 33}, {"", 3, 33}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Shares = %v, want %v", got, want)
	}
	if got := Shares(map[string]int64{"": 10}); got != nil {
		t.Fatalf("no project time: %v, want nil", got)
	}
	sum := 0
	for _, s := range Shares(map[string]int64{"a": 1, "b": 1, "c": 1, "d": 1, "e": 1, "f": 1, "g": 1}) {
		sum += s.Pct
	}
	if sum != 100 {
		t.Fatalf("shares sum to %d", sum)
	}
}
