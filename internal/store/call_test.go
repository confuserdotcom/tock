package store

import (
	"reflect"
	"testing"
)

func TestRecordCall(t *testing.T) {
	s := openAt(t, dbPath(t))
	for _, now := range []int64{100_000, 105_000, 110_000, 150_000} {
		if err := s.RecordCall(now); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.Calls(0, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	// 150_000 comes 40 s after 110_000, more than CallGapMS: a new call
	if want := []Call{{100_000, 110_000}, {150_000, 150_000}}; !reflect.DeepEqual(all, want) {
		t.Fatalf("calls = %v, want %v", all, want)
	}
	for _, c := range []struct {
		from, to int64
		want     int
	}{
		{105_000, 150_001, 2},
		{120_000, 140_000, 0},
		{110_000, 120_000, 0}, // ends exactly at from
		{90_000, 100_000, 0},  // starts exactly at to
		{109_999, 110_000, 1},
	} {
		got, err := s.Calls(c.from, c.to)
		if err != nil || len(got) != c.want {
			t.Errorf("Calls(%d, %d) = %v, %v; want %d", c.from, c.to, got, err, c.want)
		}
	}
}
