package web

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
)

type rulesResp struct {
	Rules []struct {
		File        string `json:"file"`
		Index       int    `json:"index"`
		Fingerprint string `json:"fingerprint"`
		Learned     bool   `json:"learned"`
		URL         string `json:"url"`
		Title       string `json:"title"`
		Category    string `json:"category"`
		Project     string `json:"project"`
		Kind        string `json:"kind"`
		DecidedMS   int64  `json:"decidedMS"`
		Status      string `json:"status"`
	} `json:"rules"`
}

func (f *fixture) rules() rulesResp {
	f.t.Helper()
	code, _, b := f.do("GET", "/api/rules", testToken, nil)
	var r rulesResp
	if code != 200 || json.Unmarshal(b, &r) != nil {
		f.t.Fatalf("rules: %d %s", code, b)
	}
	return r
}

func TestRulesAPI(t *testing.T) {
	f := newFixture(t)
	r := f.rules()
	if len(r.Rules) != 3 || r.Rules[0].File != "rules.toml" || r.Rules[0].Index != 1 {
		t.Fatalf("rules %+v", r.Rules)
	}
	yt := r.Rules[2]
	if yt.Category != "Entertainment" || yt.Kind != "distract" || yt.DecidedMS != 12*60_000 || yt.Status != "" {
		t.Fatalf("youtube rule %+v", yt)
	}

	rule := map[string]string{"url": `chatgpt\.com`, "label": "Study"}
	code, _, b := f.do("POST", "/api/rules/preview", testToken, map[string]any{"op": "add", "rule": rule})
	if code != 200 || !strings.Contains(string(b), `"changedMS":720000`) || !strings.Contains(string(b), `"chatgpt.com"`) {
		t.Fatalf("preview: %d %s", code, b)
	}
	code, _, b = f.do("POST", "/api/rules", testToken, map[string]any{"op": "add", "rule": rule})
	if code != 200 || !strings.Contains(string(b), `"undo":true`) {
		t.Fatalf("add: %d %s", code, b)
	}
	r = f.rules()
	first := r.Rules[0]
	if first.URL != `chatgpt\.com` || first.Category != "Study" {
		t.Fatalf("first rule %+v", first)
	}

	stale := map[string]any{"op": "set", "file": first.File, "index": first.Index, "fingerprint": "old", "rule": rule}
	if code, _, b = f.do("POST", "/api/rules", testToken, stale); code != 409 {
		t.Fatalf("stale edit: %d %s", code, b)
	}
	bad := map[string]any{"op": "add", "rule": map[string]string{"url": "([", "label": "Study"}}
	if code, _, b = f.do("POST", "/api/rules", testToken, bad); code != 400 {
		t.Fatalf("bad regex: %d %s", code, b)
	}
	del := map[string]any{"op": "remove", "file": first.File, "index": first.Index, "fingerprint": first.Fingerprint}
	if code, _, b = f.do("POST", "/api/rules", testToken, del); code != 200 {
		t.Fatalf("remove: %d %s", code, b)
	}
	if code, _, b = f.do("POST", "/api/undo", testToken, map[string]any{}); code != 200 {
		t.Fatalf("undo: %d %s", code, b)
	}
	if got := f.rules().Rules[0]; got.URL != `chatgpt\.com` {
		t.Fatalf("undo did not bring the rule back: %+v", got)
	}
}

func TestInvoiceAPI(t *testing.T) {
	f := newFixture(t)
	day := time.UnixMilli(f.library().NowMS).In(apptest.Zone).Format(dateLayout)
	q := "/api/invoice?project=tock&from=" + day + "&to=" + day
	if code, _, b := f.do("GET", q, testToken, nil); code != 400 || !strings.Contains(string(b), "no rate") {
		t.Fatalf("no rate: %d %s", code, b)
	}
	if code, _, b := f.do("POST", "/api/library/project", testToken, map[string]any{"op": "set", "name": "tock", "rate": 60}); code != 200 {
		t.Fatalf("set rate: %d %s", code, b)
	}
	code, _, b := f.do("GET", q+"&preview=1", testToken, nil)
	if code != 200 || !strings.Contains(string(b), `"seconds":6000`) || !strings.Contains(string(b), `"total":100`) {
		t.Fatalf("preview: %d %s", code, b)
	}
	code, h, b := f.do("GET", q+"&client=Acme", testToken, nil)
	if code != 200 || h.Get("Content-Type") != "application/pdf" || !strings.HasPrefix(string(b), "%PDF") ||
		!strings.Contains(h.Get("Content-Disposition"), "invoice-draft-") || !strings.Contains(string(b), "(DRAFT) Tj") {
		t.Fatalf("pdf: %d %v %.40q", code, h, b)
	}
	for _, bad := range []string{"/api/invoice?project=tock", "/api/invoice?project=nope&from=" + day + "&to=" + day,
		"/api/invoice?project=tock&from=2026-01-01&to=2027-06-01"} {
		if code, _, b := f.do("GET", bad, testToken, nil); code != 400 {
			t.Errorf("%s: %d %s", bad, code, b)
		}
	}
}
