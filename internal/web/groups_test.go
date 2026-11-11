package web

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
)

// AC6: with Billing off the API sends no rates, the invoice preview is
// refused and a rate sent with a project edit is not written. G5: status
// says which groups are on.
func TestGroupsAPI(t *testing.T) {
	f := newFixture(t)
	var st statusJSON
	f.get("/api/status", &st)
	if !st.Focus || !st.Billing {
		t.Fatalf("status %+v, want both groups on", st)
	}
	if code, _, b := f.do("POST", "/api/library/project", testToken, map[string]any{"op": "set", "name": "tock", "rate": 60}); code != 200 {
		t.Fatalf("set rate: %d %s", code, b)
	}
	f.e.WriteConfig(t, "[billing]\nenabled = false\n\n[focus]\nenabled = false\n")
	st = statusJSON{}
	f.get("/api/status", &st)
	if st.Focus || st.Billing {
		t.Fatalf("status %+v, want both groups off", st)
	}
	_, _, b := f.do("GET", "/api/library", testToken, nil)
	if strings.Contains(string(b), `"rate"`) || !strings.Contains(string(b), `"billing":false`) {
		t.Fatalf("library with billing off: %s", b)
	}
	day := time.UnixMilli(f.library().NowMS).In(apptest.Zone).Format(dateLayout)
	if code, _, b := f.do("GET", "/api/invoice?project=tock&preview=1&from="+day+"&to="+day, testToken, nil); code != 400 || !strings.Contains(string(b), "billing is off") {
		t.Fatalf("invoice with billing off: %d %s", code, b)
	}
	_, _, b = f.do("GET", "/api/export?format=json&from="+day+"&to="+day, testToken, nil)
	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err != nil || len(rows) == 0 {
		t.Fatalf("export: %v %s", err, b)
	}
	for _, r := range rows {
		if r["rate"] != nil || r["amount"] != nil {
			t.Fatalf("export row priced with billing off: %v", r)
		}
	}
	if code, _, b := f.do("POST", "/api/library/project", testToken, map[string]any{"op": "set", "name": "tock", "rate": 90}); code != 200 {
		t.Fatalf("edit with billing off: %d %s", code, b)
	}
	f.e.WriteConfig(t, "")
	_, _, b = f.do("GET", "/api/library", testToken, nil)
	if !strings.Contains(string(b), `"rate":60`) {
		t.Fatalf("rate changed while billing was off: %s", b)
	}
}
