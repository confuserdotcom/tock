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
	// a minute later the list reloads; a failed load keeps the last list
	list, fail = config.Privacy{}, errors.New("broken")
	s = span.Sample{TS: 62_000, BundleID: "diary", Title: str("x")}
	h.apply(&s)
	if loads != 2 || !s.Private {
		t.Fatalf("after a failed reload: loads %d, sample %+v", loads, s)
	}
	fail = nil
	s = span.Sample{TS: 123_000, BundleID: "diary", Title: str("x")}
	h.apply(&s)
	if s.Private {
		t.Fatalf("after an emptied list the app is still hidden: %+v", s)
	}
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
