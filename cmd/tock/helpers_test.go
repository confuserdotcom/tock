package main

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tock/internal/store"
)

// testZone is a fixed zone so day boundaries are deterministic.
var testZone = time.FixedZone("T", 2*3600)

// testNow is 2026-10-10 15:00 local.
var testNow = time.Date(2026, 10, 10, 15, 0, 0, 0, testZone)

type tenv struct {
	Env
	out, errb *bytes.Buffer
	now       time.Time
}

// newTestEnv returns an Env rooted in a temp dir with the clock at testNow.
// rules is written to rules.toml unless empty.
func newTestEnv(t *testing.T, rules string) *tenv {
	t.Helper()
	root := t.TempDir()
	te := &tenv{out: &bytes.Buffer{}, errb: &bytes.Buffer{}, now: testNow}
	te.Env = Env{
		Stdin: strings.NewReader(""), Stdout: te.out, Stderr: te.errb,
		Now:       func() time.Time { return te.now },
		DataDir:   filepath.Join(root, "data"),
		StateDir:  filepath.Join(root, "state"),
		ConfigDir: filepath.Join(root, "config"),
	}
	if rules != "" {
		if err := os.MkdirAll(te.ConfigDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(te.rulesPath(), []byte(rules), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return te
}

// run runs args with stdin and returns exit code and stdout; stderr is kept
// in te.errb.
func (te *tenv) run(t *testing.T, stdin string, args ...string) (int, string) {
	t.Helper()
	te.out.Reset()
	te.errb.Reset()
	te.Stdin = strings.NewReader(stdin)
	return run(args, te.Env), te.out.String()
}

type seedSpan struct {
	id         int64
	start, end string // "HH:MM" on testNow's day; end "" = open
	bundle     string
	app        string
	title, url string // "" = NULL
	private    bool
}

// at turns "HH:MM" into unix ms on testNow's day.
func at(t *testing.T, hhmm string) int64 {
	t.Helper()
	tm, err := time.ParseInLocation("2006-01-02 15:04", testNow.Format("2006-01-02")+" "+hhmm, testZone)
	if err != nil {
		t.Fatal(err)
	}
	return tm.UnixMilli()
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// seed creates tock.db through store.Open (so it is migrated) and inserts
// spans directly.
func (te *tenv) seed(t *testing.T, spans ...seedSpan) {
	t.Helper()
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	db, err := sql.Open("sqlite", "file:"+te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range spans {
		var end any
		if s.end != "" {
			end = at(t, s.end)
		}
		priv := 0
		if s.private {
			priv = 1
		}
		if _, err := db.Exec(`INSERT INTO span(id, start, "end", bundle_id, app, title, url, private) VALUES(?,?,?,?,?,?,?,?)`,
			s.id, at(t, s.start), end, s.bundle, s.app, nullable(s.title), nullable(s.url), priv); err != nil {
			t.Fatal(err)
		}
	}
}

const testRules = `
[[category]]
name = "Code"
kind = "deep"

[[category]]
name = "Study"
kind = "deep"

[[category]]
name = "Communication"
kind = "shallow"

[[category]]
name = "Entertainment"
kind = "distract"

[[project]]
name = "tock"

[[project]]
name = "ATLAS"

[[rule]]
bundle = ["term"]
title = '^tock │'
project = "tock"

[[rule]]
bundle = ["term"]
category = "Code"

[[rule]]
url = '^(www\.)?youtube\.com(/|$)'
category = "Entertainment"
`

// daySpans is a typical seeded day (local time):
//
//	09:00-10:40 term "tock │ ✳ x"      Code / tock   1h40
//	10:40-11:14 term "ATLAS │ y"        Code          34m
//	11:14-11:26 arc  youtube           Entertainment 12m
//	11:26-11:35 arc  chatgpt.com/c/1   unlabelled     9m
//	11:35-11:41 arc  labs.x.tv/lab/1   unlabelled     6m
//	11:41-12:22 idle                                  41m
//	12:22-12:25 arc  chatgpt.com/c/2   unlabelled     3m
//	14:50-open  mail "Inbox"           unlabelled    10m (to 15:00)
func daySpans() []seedSpan {
	return []seedSpan{
		{1, "09:00", "10:40", "term", "WezTerm", "tock │ ✳ x", "", false},
		{2, "10:40", "11:14", "term", "WezTerm", "ATLAS │ y", "", false},
		{3, "11:14", "11:26", "arc", "Arc", "Video", "www.youtube.com/watch", false},
		{4, "11:26", "11:35", "arc", "Arc", "XOR Equivalence Check", "chatgpt.com/c/1", false},
		{5, "11:35", "11:41", "arc", "Arc", "Low Level Labs", "labs.x.tv/lab/1", false},
		{6, "11:41", "12:22", "tock.idle", "Idle", "", "", false},
		{7, "12:22", "12:25", "arc", "Arc", "Proofs", "chatgpt.com/c/2", false},
		{8, "14:50", "", "mail", "Mail", "Inbox", "", false},
	}
}
