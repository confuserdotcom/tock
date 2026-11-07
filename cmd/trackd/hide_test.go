package main

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"tock/internal/config"
	"tock/internal/span"
	"tock/internal/store"
)

func str(s string) *string { return &s }

func TestHiderApply(t *testing.T) {
	loads := 0
	list := config.Privacy{HideApps: []string{"diary"}, HideSites: []string{"bank.com"}}
	var fail error
	h := &hider{every: 60_000, load: func() (config.Privacy, error) { loads++; return list, fail }}

	s := span.Sample{TS: 1000, BundleID: "diary", App: "Diary", Title: str("Dear diary")}
	h.apply(&s)
	if !s.Private || s.Title != nil || s.URL != nil || s.App != "Diary" {
		t.Fatalf("hidden app sample = %+v", s)
	}
	s = span.Sample{TS: 2000, BundleID: "safari", Title: str("Account"), URL: str("www.bank.com/acct")}
	h.apply(&s)
	if !s.Private || s.Title != nil || s.URL != nil {
		t.Fatalf("hidden site sample = %+v", s)
	}
	s = span.Sample{TS: 3000, BundleID: "safari", Title: str("News"), URL: str("news.com/")}
	h.apply(&s)
	if s.Private || s.Title == nil || s.URL == nil {
		t.Fatalf("visible sample changed: %+v", s)
	}
	if loads != 1 {
		t.Fatalf("loads = %d within a minute, want 1", loads)
	}
	// a minute later the list reloads; an emptied list hides nothing
	list = config.Privacy{}
	s = span.Sample{TS: 62_000, BundleID: "diary", Title: str("x")}
	h.apply(&s)
	if loads != 2 || s.Private {
		t.Fatalf("after an emptied list: loads %d, sample %+v", loads, s)
	}
	_ = fail
	var nilH *hider
	nilH.apply(&s) // a nil hider changes nothing
}

func TestPipeHides(t *testing.T) {
	const t0 = int64(1_700_000_000_000)
	path := filepath.Join(t.TempDir(), "tock.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	in := make(chan span.Input, 4)
	h := &hider{every: 60_000, load: func() (config.Privacy, error) {
		return config.Privacy{HideApps: []string{"diary"}}, nil
	}}
	p := pipe{in: in, m: span.New(span.DefaultConfig()), st: st, now: func() int64 { return t0 }, hide: h}
	in <- span.Input{Sample: &span.Sample{TS: t0, BundleID: "diary", App: "Diary", Title: str("secret")}}
	in <- span.Input{Sample: &span.Sample{TS: t0 + 2000, BundleID: "b", App: "B", Title: str("open")}}
	close(in)
	if err := p.run(); err != nil {
		t.Fatal(err)
	}
	st.Close()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var title sql.NullString
	var private int
	if err := db.QueryRow(`SELECT title, private FROM span WHERE bundle_id = 'diary'`).Scan(&title, &private); err != nil {
		t.Fatal(err)
	}
	if title.Valid || private != 1 {
		t.Fatalf("hidden span: title %v, private %d", title, private)
	}
}

// A config that does not load pauses capture: no defaults, no last good
// list. The settings are read again every hidePausedEvery while paused, and
// capture resumes once they load.
func TestHiderPausesOnLoadError(t *testing.T) {
	list := config.Privacy{HideApps: []string{"diary"}}
	fail := errors.New("config.toml: toml: line 3: expected '='")
	loads := 0
	h := &hider{every: 60_000, load: func() (config.Privacy, error) { loads++; return list, fail }}

	if changed := h.check(1000); !changed || !h.paused() {
		t.Fatalf("first load failed: changed %v, paused %v", changed, h.paused())
	}
	if h.reason() != fail.Error() {
		t.Fatalf("reason = %q", h.reason())
	}
	if changed := h.check(1000 + hidePausedEvery - 1); changed || loads != 1 {
		t.Fatalf("reread too soon: changed %v, loads %d", changed, loads)
	}
	fail = nil
	if changed := h.check(1000 + hidePausedEvery); !changed || h.paused() || loads != 2 {
		t.Fatalf("after the fix: changed %v, paused %v, loads %d", changed, h.paused(), loads)
	}
	s := span.Sample{TS: 7000, BundleID: "diary", Title: str("x")}
	h.apply(&s)
	if !s.Private {
		t.Fatalf("hidden app after resume: %+v", s)
	}

	// a good list that later fails to load pauses too: no "last good" kept
	fail = errors.New("broken")
	if changed := h.check(7000 + 60_000); !changed || !h.paused() {
		t.Fatalf("a failed reload must pause: changed %v, paused %v", changed, h.paused())
	}

	var nilH *hider
	if nilH.check(1) || nilH.paused() {
		t.Fatal("a nil hider never pauses")
	}
}
