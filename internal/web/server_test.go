package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"tock/internal/app"
	"tock/internal/apptest"
)

const testToken = "test-token"

type fixture struct {
	t   *testing.T
	srv *httptest.Server
	e   *apptest.Env
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a, err := app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config, ShellLog: e.ShellLog}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	srv := httptest.NewUnstartedServer(nil)
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	srv.Config.Handler = New(a, testToken, port, e.NowFunc()).Handler()
	srv.Start()
	t.Cleanup(srv.Close)
	return &fixture{t, srv, e}
}

// do sends a request with the token unless token is "-".
func (f *fixture) do(method, path, token string, body any) (int, http.Header, []byte) {
	f.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, f.srv.URL+path, r)
	if token != "-" {
		req.Header.Set(TokenHeader, token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, b
}

func (f *fixture) get(path string, v any) {
	f.t.Helper()
	code, _, b := f.do("GET", path, testToken, nil)
	if code != 200 {
		f.t.Fatalf("GET %s: %d %s", path, code, b)
	}
	if err := json.Unmarshal(b, v); err != nil {
		f.t.Fatalf("GET %s: %v", path, err)
	}
}

func TestPageNeedsNoTokenAndCarriesHeaders(t *testing.T) {
	f := newFixture(t)
	for _, p := range []string{"/", "/app.js", "/app.css"} {
		code, h, b := f.do("GET", p, "-", nil)
		if code != 200 || len(b) == 0 {
			t.Fatalf("%s: %d", p, code)
		}
		if !strings.Contains(h.Get("Content-Security-Policy"), "script-src 'self'") || h.Get("X-Frame-Options") != "DENY" {
			t.Fatalf("%s: headers %v", p, h)
		}
	}
}

func TestAPINeedsToken(t *testing.T) {
	f := newFixture(t)
	for _, tok := range []string{"-", "", "wrong"} {
		if code, _, _ := f.do("GET", "/api/day", tok, nil); code != http.StatusUnauthorized {
			t.Fatalf("token %q: %d", tok, code)
		}
	}
	if code, h, _ := f.do("GET", "/api/day", testToken, nil); code != 200 || h.Get("Cache-Control") != "no-store" {
		t.Fatalf("good token: %d %v", code, h)
	}
}

func TestWrongHostRefused(t *testing.T) {
	f := newFixture(t)
	req, _ := http.NewRequest("GET", f.srv.URL+"/api/day", nil)
	req.Host = "evil.example:" + strconv.Itoa(f.srv.Listener.Addr().(*net.TCPAddr).Port)
	req.Header.Set(TokenHeader, testToken)
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestPostNeedsJSON(t *testing.T) {
	f := newFixture(t)
	req, _ := http.NewRequest("POST", f.srv.URL+"/api/ignore", strings.NewReader(`{"key":"x"}`))
	req.Header.Set(TokenHeader, testToken)
	req.Header.Set("Content-Type", "text/plain")
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status %d", resp.StatusCode)
	}
	big := map[string]string{"key": strings.Repeat("x", maxBody)}
	if code, _, _ := f.do("POST", "/api/ignore", testToken, big); code != http.StatusBadRequest {
		t.Fatalf("big body: %d", code)
	}
}

func TestDayWeekTop(t *testing.T) {
	f := newFixture(t)
	var d dayJSON
	f.get("/api/day", &d)
	if d.Date != "2026-10-10" || !d.Today || len(d.Timeline) == 0 || d.TrackedMS == 0 || len(d.Categories) == 0 {
		t.Fatalf("day %+v", d)
	}
	if last := d.Timeline[len(d.Timeline)-1]; !last.Open || last.App != "Mail" {
		t.Fatalf("last row %+v", last)
	}
	if len(d.Blocks) == 0 || d.Blocks[0].Project != "tock" || d.Blocks[0].Category != "Code" || len(d.Blocks[0].Top) == 0 {
		t.Fatalf("blocks %+v", d.Blocks)
	}
	var past dayJSON
	f.get("/api/day?date=2026-10-09", &past)
	if past.Today || past.TrackedMS != 0 {
		t.Fatalf("past %+v", past)
	}
	if code, _, _ := f.do("GET", "/api/day?date=tomorrow", testToken, nil); code != 400 {
		t.Fatalf("bad date: %d", code)
	}
	var w weekJSON
	f.get("/api/week", &w)
	if w.Monday != "2026-10-05" || len(w.Days) != 7 || w.TrackedMS != d.TrackedMS {
		t.Fatalf("week %+v", w)
	}
	var top topJSON
	f.get("/api/top?range=today", &top)
	if top.From != "2026-10-10" || len(top.Sections["category"]) == 0 || top.Sections["site"] == nil {
		t.Fatalf("top %+v", top)
	}
	if code, _, _ := f.do("GET", "/api/top?days=0", testToken, nil); code != 400 {
		t.Fatalf("days=0: %d", code)
	}
}

func inboxKeys(f *fixture) []string {
	var in inboxJSON
	f.get("/api/inbox", &in)
	keys := make([]string, len(in.Items))
	for i, it := range in.Items {
		keys[i] = it.Key
	}
	return keys
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func TestTagAndIgnore(t *testing.T) {
	f := newFixture(t)
	keys := inboxKeys(f)
	if len(keys) < 2 {
		t.Fatalf("inbox %v", keys)
	}
	var done doneJSON
	code, _, b := f.do("POST", "/api/tag", testToken, map[string]any{"key": keys[0], "label": "stu/atl"})
	if code != 200 || json.Unmarshal(b, &done) != nil || done.Label != "Study / ATLAS" || done.Spans == 0 {
		t.Fatalf("tag: %d %s", code, b)
	}
	if code, _, b := f.do("POST", "/api/tag", testToken, map[string]any{"key": keys[1], "label": "Nope"}); code != 400 ||
		!strings.Contains(string(b), "unknown category") {
		t.Fatalf("bad label: %d %s", code, b)
	}
	if code, _, _ := f.do("POST", "/api/ignore", testToken, map[string]any{"key": keys[1]}); code != 200 {
		t.Fatalf("ignore: %d", code)
	}
	after := inboxKeys(f)
	if contains(after, keys[0]) || contains(after, keys[1]) {
		t.Fatalf("inbox still has tagged or ignored keys: %v", after)
	}
	if code, _, _ := f.do("POST", "/api/ignore", testToken, map[string]any{"key": keys[1], "undo": true}); code != 200 ||
		!contains(inboxKeys(f), keys[1]) {
		t.Fatal("undo ignore")
	}
}

func TestTagTimelineRow(t *testing.T) {
	f := newFixture(t)
	var d dayJSON
	f.get("/api/day", &d)
	row := d.Timeline[len(d.Timeline)-1]
	if code, _, b := f.do("POST", "/api/tag", testToken, map[string]any{"ids": row.IDs, "label": "Communication"}); code != 200 {
		t.Fatalf("tag row: %d %s", code, b)
	}
	f.get("/api/day", &d)
	if got := d.Timeline[len(d.Timeline)-1]; got.Category != "Communication" || got.Source != "manual" {
		t.Fatalf("row %+v", got)
	}
}

func TestProposeAndLearn(t *testing.T) {
	f := newFixture(t)
	var key string
	for _, k := range inboxKeys(f) {
		if strings.Contains(k, "chatgpt.com") {
			key = k
		}
	}
	if key == "" {
		t.Fatal("no chatgpt key")
	}
	var p proposeJSON
	f.get("/api/propose?key="+url.QueryEscape(key)+"&label=Study", &p)
	if len(p.Candidates) == 0 || p.Label != "Study" || p.Spans != 2 {
		t.Fatalf("propose %+v", p)
	}
	if code, _, _ := f.do("POST", "/api/learn", testToken, map[string]any{"key": key, "label": "Study", "rule": "app = \"nope\""}); code != 409 {
		t.Fatalf("stale rule: %d", code)
	}
	if code, _, b := f.do("POST", "/api/learn", testToken, map[string]any{"key": key, "label": "Study", "rule": p.Candidates[0].Rule}); code != 200 {
		t.Fatalf("learn: %d %s", code, b)
	}
	if contains(inboxKeys(f), key) {
		t.Fatal("learned key still in inbox")
	}
	var l labelsJSON
	f.get("/api/labels", &l)
	if len(l.Categories) != 4 || contains(l.Projects, "Old") || !contains(l.Projects, "tock") {
		t.Fatalf("labels %+v", l)
	}
}

func TestEditAndUndo(t *testing.T) {
	f := newFixture(t)
	var before dayJSON
	f.get("/api/day", &before)
	r := before.Timeline[0]
	if code, _, b := f.do("POST", "/api/edit", testToken, map[string]any{"kind": "cut", "from": r.Start, "to": r.End}); code != 200 {
		t.Fatalf("cut: %d %s", code, b)
	}
	var after dayJSON
	f.get("/api/day", &after)
	if after.TrackedMS != before.TrackedMS-(r.End-r.Start) {
		t.Fatalf("tracked %d, was %d", after.TrackedMS, before.TrackedMS)
	}
	if code, _, b := f.do("POST", "/api/edit", testToken, map[string]any{"kind": "add", "from": r.Start, "to": r.End, "label": "Nope"}); code != 400 {
		t.Fatalf("bad label: %d %s", code, b)
	}
	if code, _, b := f.do("POST", "/api/edit", testToken, map[string]any{"kind": "cut", "from": r.End, "to": r.Start}); code != 400 {
		t.Fatalf("bad range: %d %s", code, b)
	}
	code, _, b := f.do("POST", "/api/undo", testToken, map[string]any{})
	if code != 200 || !strings.Contains(string(b), `"what":"delete`) {
		t.Fatalf("undo: %d %s", code, b)
	}
	f.get("/api/day", &after)
	if after.TrackedMS != before.TrackedMS {
		t.Fatal("undo did not restore the time")
	}
	if _, _, b := f.do("POST", "/api/undo", testToken, map[string]any{}); string(b) != `{"what":""}`+"\n" && string(b) != `{"what":""}` {
		t.Fatalf("empty undo: %s", b)
	}
}
