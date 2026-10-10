package report

import (
	"reflect"
	"testing"
)

func TestCommits(t *testing.T) {
	rows := Commits(map[string]int{"tock": 4, "ATLAS": 1, "zed": 4, "none": 0})
	want := []NameN{{"tock", 4}, {"zed", 4}, {"ATLAS", 1}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows = %v", rows)
	}
	if got := CommitsLine(rows); got != "tock 4 · zed 4 · ATLAS 1" {
		t.Fatalf("line = %q", got)
	}
	if Commits(nil) != nil || CommitsLine(nil) != "" {
		t.Fatal("empty")
	}
}
