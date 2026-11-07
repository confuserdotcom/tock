package snapshot

import (
	"slices"
	"testing"
)

const h = int64(3600 * 1000)

func names(is []Info) []string {
	var out []string
	for _, i := range is {
		out = append(out, i.Name)
	}
	slices.Sort(out)
	return out
}

func TestPlan(t *testing.T) {
	now := 1000 * 24 * h
	d := func(name string, ageH int64) Info { return Info{Name: name, Kind: Daily, AtMS: now - ageH*h} }
	p := func(name string, ageH int64) Info { return Info{Name: name, Kind: Pre, AtMS: now - ageH*h} }
	cases := []struct {
		name string
		in   []Info
		keep int
		want []string
	}{
		{"empty", nil, 7, nil},
		{"under keep", []Info{d("d1", 1), d("d2", 25)}, 7, nil},
		{"over keep drops oldest", []Info{d("d1", 1), d("d2", 25), d("d3", 49)}, 2, []string{"d3"}},
		{"keep 0 leaves dailies", []Info{d("d1", 1), d("d2", 500)}, 0, nil},
		{"old pre dropped, newest pre kept", []Info{p("p1", 15*24), p("p2", 20*24)}, 7, []string{"p2"}},
		{"young pre kept", []Info{p("p1", 1), p("p2", 13*24)}, 7, nil},
	}
	for _, c := range cases {
		if got := names(Plan(c.in, now, c.keep)); !slices.Equal(got, c.want) {
			t.Errorf("%s: remove %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDue(t *testing.T) {
	now := 1000 * h
	cases := []struct {
		newest int64
		away   bool
		want   bool
	}{
		{0, false, true},
		{now - 23*h, true, false},
		{now - 24*h, false, false},
		{now - 24*h, true, true},
		{now - 36*h, false, true},
	}
	for _, c := range cases {
		if got := Due(c.newest, now, c.away); got != c.want {
			t.Errorf("Due(%d, away=%v) = %v, want %v", now-c.newest, c.away, got, c.want)
		}
	}
}
