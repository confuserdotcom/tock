package app

import (
	"slices"
	"testing"

	"tock/internal/apptest"
)

func TestViewsCarryScreenWords(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	if err := a.st.SetSpanWords(8, []string{"invoice", "client"}, a.nowMS()); err != nil {
		t.Fatal(err)
	}
	from := a.midnight(a.now()).UnixMilli()
	vs, _, err := a.labelled(from, a.nowMS(), a.nowMS())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range vs {
		switch v.ID {
		case 8:
			found = slices.Equal(v.Screen, []string{"invoice", "client"})
		default:
			if v.Screen != nil {
				t.Errorf("view %d has screen words %v", v.ID, v.Screen)
			}
		}
	}
	if !found {
		t.Fatal("span 8 lost its screen words")
	}
}
