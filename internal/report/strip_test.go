package report

import (
	"reflect"
	"testing"
)

func TestStrip(t *testing.T) {
	if got := Strip(nil, 24); got != nil {
		t.Fatalf("empty = %v", got)
	}
	if got := Strip(map[string]int64{"deep": 0}, 24); got != nil {
		t.Fatalf("zero = %v", got)
	}
	if got := Strip(map[string]int64{"deep": 5}, 4); !reflect.DeepEqual(got, []string{"deep", "deep", "deep", "deep"}) {
		t.Fatalf("one kind = %v", got)
	}
	// 1:1:2 over 4 cells, in Kinds order
	got := Strip(map[string]int64{"distract": 20, "deep": 10, "shallow": 10}, 4)
	if want := []string{"deep", "shallow", "distract", "distract"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("mix = %v, want %v", got, want)
	}
	// shares 1.5/0.9/0.6: floors 1/0/0, the two spare cells go to the largest remainders
	got = Strip(map[string]int64{"deep": 50, "shallow": 30, "neutral": 20}, 3)
	if want := []string{"deep", "shallow", "neutral"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("remainders = %v, want %v", got, want)
	}
	if n := len(Strip(map[string]int64{"deep": 7, "": 3, "browse": 1}, 24)); n != 24 {
		t.Fatalf("len = %d", n)
	}
	if got := Strip(map[string]int64{"idle": 9}, 4); got != nil {
		t.Fatalf("unknown kind only = %v", got)
	}
}
