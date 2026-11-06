package web

import (
	"encoding/json"
	"strings"
	"testing"
)

type libResp struct {
	Categories []struct {
		Name       string `json:"name"`
		Kind       string `json:"kind"`
		Interrupt  bool   `json:"interrupt"`
		Projects   int    `json:"projects"`
		Last30MS   int64  `json:"last30MS"`
		LastSeenMS int64  `json:"lastSeenMS"`
	} `json:"categories"`
	Projects []struct {
		Name     string   `json:"name"`
		Rate     *float64 `json:"rate"`
		Archived bool     `json:"archived"`
		Last30MS int64    `json:"last30MS"`
	} `json:"projects"`
	Goals []struct {
		Name string `json:"name"`
		MS   int64  `json:"ms"`
	} `json:"goals"`
	Currency string `json:"currency"`
	NowMS    int64  `json:"nowMS"`
}

func (f *fixture) library() libResp {
	f.t.Helper()
	code, _, b := f.do("GET", "/api/library", testToken, nil)
	var r libResp
	if code != 200 || json.Unmarshal(b, &r) != nil {
		f.t.Fatalf("library: %d %s", code, b)
	}
	return r
}

func TestLibraryAPI(t *testing.T) {
	f := newFixture(t)
	r := f.library()
	if len(r.Categories) == 0 || len(r.Projects) == 0 || r.NowMS == 0 {
		t.Fatalf("library %+v", r)
	}
	for _, p := range r.Projects {
		if p.Name == "tock" && p.Last30MS != 100*60_000 {
			t.Errorf("tock last30 %d", p.Last30MS)
		}
	}

	post := func(path string, body any) (int, map[string]any) {
		code, _, b := f.do("POST", path, testToken, body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		return code, m
	}

	code, m := post("/api/library/category", map[string]any{"op": "add", "name": "Reading", "kind": "neutral"})
	if code != 200 || m["undo"] != true || !strings.Contains(m["what"].(string), "Reading") {
		t.Fatalf("add category: %d %v", code, m)
	}
	if code, m = post("/api/undo", map[string]any{}); code != 200 || !strings.Contains(m["what"].(string), "Reading") {
		t.Fatalf("undo: %d %v", code, m)
	}

	if code, m = post("/api/library/project", map[string]any{"op": "set", "name": "tock", "rate": 45}); code != 200 {
		t.Fatalf("rate: %d %v", code, m)
	}
	for _, p := range f.library().Projects {
		if p.Name == "tock" && (p.Rate == nil || *p.Rate != 45) {
			t.Errorf("tock rate %v", p.Rate)
		}
	}

	if code, m = post("/api/library/project", map[string]any{"op": "rename", "name": "ATLAS", "new": "Atlas"}); code != 200 || m["undo"] != false {
		t.Fatalf("rename: %d %v", code, m)
	}

	goal := map[string]any{"category": "Study", "week": true, "ms": 6 * 3_600_000}
	if code, m = post("/api/library/goal", map[string]any{"op": "set", "goal": goal}); code != 200 {
		t.Fatalf("goal: %d %v", code, m)
	}
	goal2 := map[string]any{"category": "Study", "week": false, "ms": 3_600_000}
	if code, m = post("/api/library/goal", map[string]any{"op": "set", "goal": goal2, "was": goal}); code != 200 {
		t.Fatalf("goal edit: %d %v", code, m)
	}
	gs := f.library().Goals
	n := 0
	for _, g := range gs {
		if g.Name == "Study" {
			n++
			if g.MS != 3_600_000 {
				t.Errorf("study goal ms %d", g.MS)
			}
		}
	}
	if n != 1 {
		t.Errorf("study goals %d, want the edited one only", n)
	}

	for _, bad := range []struct {
		path string
		body map[string]any
	}{
		{"/api/library/category", map[string]any{"op": "add", "name": "X", "kind": "loud"}},
		{"/api/library/category", map[string]any{"op": "fly", "name": "Code"}},
		{"/api/library/project", map[string]any{"op": "set", "name": "tock", "rate": 1e9}},
		{"/api/library/project", map[string]any{"op": "set", "name": "nope"}},
		{"/api/library/merge", map[string]any{"kind": "app", "from": "a", "into": "b"}},
	} {
		if code, m := post(bad.path, bad.body); code != 400 || m["error"] == "" {
			t.Errorf("%s %v: %d %v", bad.path, bad.body, code, m)
		}
	}
	if code, _, _ := f.do("GET", "/api/library", "-", nil); code != 401 {
		t.Fatalf("no token: %d", code)
	}
}
