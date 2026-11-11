package web

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/apptest"
)

// With Billing off the API sends no rates, the invoice preview is
// refused and a rate sent with a project edit is not written. Status
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

// The day and the week mark billed time; relabelling it says the
// invoice stays as issued.
func TestBilledMarkers(t *testing.T) {
	f := newFixture(t)
	f.withIssuer(t)
	if code, _, b := f.do("POST", "/api/library/project", testToken, map[string]any{"op": "set", "name": "tock", "rate": 60}); code != 200 {
		t.Fatalf("set rate: %d %s", code, b)
	}
	now := time.UnixMilli(f.library().NowMS).In(apptest.Zone)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	inv, err := f.a.IssueInvoice(app.InvoiceSpec{Project: "tock", From: day, To: day})
	if err != nil {
		t.Fatal(err)
	}
	var d dayJSON
	f.get("/api/day", &d)
	var billed []int64
	for _, r := range d.Timeline {
		switch {
		case r.Project == "tock" && r.Invoice != inv.Number:
			t.Fatalf("tock row not marked: %+v", r)
		case r.Project != "tock" && r.Invoice != "":
			t.Fatalf("row wrongly marked: %+v", r)
		case r.Invoice != "":
			billed = append(billed, r.IDs...)
		}
	}
	if len(billed) == 0 || d.BilledMS != 6000_000 {
		t.Fatalf("billed ids %v, billedMS %d", billed, d.BilledMS)
	}
	var w weekJSON
	f.get("/api/week", &w)
	var wms int64
	for _, x := range w.Days {
		wms += x.BilledMS
	}
	if wms != d.BilledMS {
		t.Fatalf("week billed %d, day %d", wms, d.BilledMS)
	}
	code, _, b := f.do("POST", "/api/tag", testToken, map[string]any{"ids": billed[:1], "label": "Code"})
	if code != 200 || !strings.Contains(string(b), "On invoice "+inv.Number+"; the invoice stays as issued.") {
		t.Fatalf("tag billed: %d %s", code, b)
	}
}

// Learning a rule that relabels billed time says so too, as tagging does.
func TestLearnBilledNote(t *testing.T) {
	f := newFixture(t)
	f.withIssuer(t)
	if code, _, b := f.do("POST", "/api/library/project", testToken, map[string]any{"op": "set", "name": "tock", "rate": 60}); code != 200 {
		t.Fatalf("set rate: %d %s", code, b)
	}
	now := time.UnixMilli(f.library().NowMS).In(apptest.Zone)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	inv, err := f.a.IssueInvoice(app.InvoiceSpec{Project: "tock", From: day, To: day})
	if err != nil {
		t.Fatal(err)
	}
	var d dayJSON
	f.get("/api/day", &d)
	key := ""
	for _, r := range d.Timeline {
		if r.Invoice != "" {
			key = r.Key
			break
		}
	}
	if key == "" {
		t.Fatal("no billed row")
	}
	var p proposeJSON
	f.get("/api/propose?key="+url.QueryEscape(key)+"&label=Study", &p)
	rule := ""
	for _, c := range p.Candidates {
		rule = c.Rule
		break
	}
	if rule == "" {
		t.Fatalf("no safe rule for %q: %+v", key, p)
	}
	code, _, b := f.do("POST", "/api/learn", testToken, map[string]any{"key": key, "label": "Study", "rule": rule})
	if code != 200 || !strings.Contains(string(b), "On invoice "+inv.Number) {
		t.Fatalf("learn on billed time: %d %s", code, b)
	}
}
