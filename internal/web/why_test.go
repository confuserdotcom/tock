package web

import (
	"strconv"
	"testing"

	"tock/internal/apptest"
)

func TestWhyEndpoint(t *testing.T) {
	f := newFixture(t)
	from, to := apptest.At(t, "09:00", 0), apptest.At(t, "10:40", 0)
	var w struct {
		Lines []whyJSON `json:"lines"`
	}
	f.get("/api/why?id=1&from="+strconv.FormatInt(from, 10)+"&to="+strconv.FormatInt(to, 10), &w)
	if len(w.Lines) != 2 || w.Lines[0].Source != "rule" || w.Lines[0].Ref != "rules.toml#2" || w.Lines[1].Label != "tock" {
		t.Fatalf("lines %+v", w.Lines)
	}
	for _, q := range []string{"id=x&from=1&to=2", "id=1&from=5&to=1", "id=1&from=0&to=999999999999"} {
		if code, _, _ := f.do("GET", "/api/why?"+q, testToken, nil); code != 400 {
			t.Errorf("%s: %d", q, code)
		}
	}
}
