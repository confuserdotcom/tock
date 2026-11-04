package main

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"tock/internal/classify"
	"tock/internal/collect"
	"tock/internal/config"
	"tock/internal/span"
	"tock/internal/store"
)

type fakeCapturer struct {
	calls   []string
	result  collect.OCRResult
	granted bool
}

func (f *fakeCapturer) Capture(_ context.Context, bundle string) collect.OCRResult {
	f.calls = append(f.calls, bundle)
	return f.result
}

func (f *fakeCapturer) Grant(context.Context) (bool, error) {
	f.calls = append(f.calls, "grant")
	return f.granted, nil
}

type ocrRig struct {
	c     *ocrCoord
	cap   *fakeCapturer
	st    *store.Store
	now   int64
	cfg   config.Config
	sec   bool
	logs  *bytes.Buffer
	rules string
}

const ocrT0 = int64(1_800_000_000_000)

func newOCRRig(t *testing.T) *ocrRig {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "tock.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	r := &ocrRig{cap: &fakeCapturer{result: collect.OCRResult{Reason: collect.OCROK, Lines: []string{"kubectl rollout kubectl"}, CostMS: 140}},
		st: st, now: ocrT0, cfg: config.Default(), logs: &bytes.Buffer{}}
	r.cfg.OCR.Enabled = true
	r.rules = `
[[category]]
name = "Code"
kind = "deep"

[[rule]]
bundle = ["term"]
title = 'nvim'
category = "Code"

[[rule]]
bundle = ["com.apple.Safari"]
category = "Code"
`
	r.c = newOCRCoord(&ocrCoord{
		cap:    r.cap,
		secure: func() bool { return r.sec },
		load: func() (ocrSettings, error) {
			rs, err := classify.Parse([]byte(r.rules), nil)
			if err != nil {
				return ocrSettings{}, err
			}
			return newOCRSettings(r.cfg, rs)
		},
		st: st, log: slog.New(slog.NewTextHandler(r.logs, nil)), now: func() int64 { return r.now },
		idleMS: 300_000, runSync: true,
	})
	return r
}

func strPtr(s string) *string { return &s }

// open starts span id of bundle at r.now.
func (r *ocrRig) open(id int64, bundle, title string) {
	sp := span.Span{ID: id, Start: r.now, BundleID: bundle, App: bundle}
	if title != "" {
		sp.Title = strPtr(title)
	}
	r.c.observe([]span.Op{{Kind: span.OpOpen, Span: sp}}, nil)
}

// sample feeds one sample of bundle at r.now + ms.
func (r *ocrRig) sample(ms int64, s span.Sample) {
	r.now += ms
	s.TS = r.now
	r.c.observe(nil, &s)
}

func words(t *testing.T, st *store.Store, id int64) []string {
	t.Helper()
	w, err := st.SpanWords([]int64{id})
	if err != nil {
		t.Fatal(err)
	}
	return w[id]
}

func stats(t *testing.T, st *store.Store, now int64) map[string]store.OCRStat {
	t.Helper()
	day := dayOf(now)
	got, err := st.OCRStats(day)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestOCRReadsAfterDwellThenThrottles(t *testing.T) {
	r := newOCRRig(t)
	r.open(1, "term", "zsh")
	s := span.Sample{BundleID: "term", App: "term", Title: strPtr("zsh")}
	r.sample(19_000, s)
	if len(r.cap.calls) != 0 {
		t.Fatal("read before min_dwell")
	}
	r.sample(1_000, s) // 20 s
	if !slices.Equal(r.cap.calls, []string{"term"}) {
		t.Fatalf("calls = %v", r.cap.calls)
	}
	if got := words(t, r.st, 1); !slices.Equal(got, []string{"kubectl", "rollout"}) {
		t.Fatalf("words = %v", got)
	}
	r.sample(59_000, s)
	if len(r.cap.calls) != 1 {
		t.Fatal("read before interval")
	}
	r.cap.result.Lines = []string{"namespace"}
	r.sample(1_000, s)
	if len(r.cap.calls) != 2 {
		t.Fatal("no second read after interval")
	}
	if got := words(t, r.st, 1); !slices.Equal(got, []string{"namespace", "kubectl", "rollout"}) {
		t.Fatalf("merged words = %v", got)
	}
	if st := stats(t, r.st, r.now); st[collect.OCROK] != (store.OCRStat{N: 2, CostMS: 280}) || len(st) != 1 {
		t.Fatalf("stats = %+v", st)
	}
	if strings.Contains(r.logs.String(), "kubectl") {
		t.Fatal("log carries screen text")
	}
}

func TestOCRSkips(t *testing.T) {
	cases := []struct {
		name   string
		bundle string
		s      span.Sample
		setup  func(r *ocrRig)
		want   string // counted skip; "" = none counted
	}{
		{"off", "term", span.Sample{Title: strPtr("zsh")}, func(r *ocrRig) { r.cfg.OCR.Enabled = false }, ""},
		{"no capturer", "term", span.Sample{Title: strPtr("zsh")}, func(r *ocrRig) { r.c.cap = nil }, collect.OCRUnavailable},
		{"away", "term", span.Sample{Title: strPtr("zsh"), IdleS: 400}, nil, collect.OCRAway},
		{"locked", "term", span.Sample{Title: strPtr("zsh"), Locked: true}, nil, collect.OCRLocked},
		{"secure input", "term", span.Sample{Title: strPtr("zsh")}, func(r *ocrRig) { r.sec = true }, collect.OCRSecure},
		{"sign-in title", "term", span.Sample{Title: strPtr("Sign in to your account")}, nil, collect.OCRSecure},
		{"llm deny app", "com.1password.1password", span.Sample{Title: strPtr("Vault")}, nil, collect.OCRDeny},
		{"ocr deny title", "term", span.Sample{Title: strPtr("diary")}, func(r *ocrRig) { r.cfg.OCR.DenyTitles = []string{"diary"} }, collect.OCRDeny},
		{"llm deny host", "com.apple.Safari", span.Sample{Title: strPtr("x"), URL: strPtr("www.paypal.com/x")}, nil, collect.OCRDeny},
		{"private", "com.apple.Safari", span.Sample{Private: true}, nil, collect.OCRPrivate},
		{"browser, unknown title", "com.apple.Safari", span.Sample{}, nil, collect.OCRPrivate},
		{"clear label", "term", span.Sample{Title: strPtr("nvim main.go")}, func(r *ocrRig) { r.cfg.OCR.Apps = nil }, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newOCRRig(t)
			if c.setup != nil {
				c.setup(r)
			}
			r.open(1, c.bundle, "")
			c.s.BundleID, c.s.App = c.bundle, c.bundle
			r.sample(30_000, c.s)
			r.sample(30_000, c.s) // the same skip counts once
			if len(r.cap.calls) != 0 {
				t.Fatalf("captured: %v", r.cap.calls)
			}
			st := stats(t, r.st, r.now)
			if c.want == "" {
				if len(st) != 0 {
					t.Fatalf("stats = %+v, want none", st)
				}
				return
			}
			if st[c.want].N != 1 || len(st) != 1 {
				t.Fatalf("stats = %+v, want one %s", st, c.want)
			}
		})
	}
}

func TestOCRAppsListReadsLabelledWindows(t *testing.T) {
	r := newOCRRig(t)
	r.open(1, "term", "nvim main.go") // labelled by a title rule, but term is in [ocr] apps
	r.cfg.OCR.Apps = []string{"term"}
	r.c.loaded = false
	r.sample(25_000, span.Sample{BundleID: "term", App: "term", Title: strPtr("nvim main.go")})
	if len(r.cap.calls) != 1 {
		t.Fatalf("calls = %v", r.cap.calls)
	}
}

func TestOCRWorkerSkipAndPermission(t *testing.T) {
	r := newOCRRig(t)
	r.cap.result = collect.OCRResult{Reason: collect.OCRNoPermission}
	r.open(1, "term", "zsh")
	s := span.Sample{BundleID: "term", App: "term", Title: strPtr("zsh")}
	r.sample(25_000, s)
	if v, _, _ := r.st.State(keyOCRPerm); v != "missing" {
		t.Fatalf("perm = %q", v)
	}
	r.sample(50_000, s) // the missing permission is trusted for a minute: no read
	r.cap.result = collect.OCRResult{Reason: collect.OCROK, Lines: []string{"pods"}}
	r.sample(30_000, s)
	if len(r.cap.calls) != 2 {
		t.Fatalf("calls = %v, want a retry after the permission wait", r.cap.calls)
	}
	if v, _, _ := r.st.State(keyOCRPerm); v != "granted" {
		t.Fatalf("perm = %q", v)
	}
	if st := stats(t, r.st, r.now); st[collect.OCRNoPermission].N != 1 || st[collect.OCROK].N != 1 {
		t.Fatalf("stats = %+v", st)
	}
}

func TestOCRBusy(t *testing.T) {
	r := newOCRRig(t)
	r.open(1, "term", "zsh")
	r.c.busy = true
	r.sample(25_000, span.Sample{BundleID: "term", App: "term", Title: strPtr("zsh")})
	if len(r.cap.calls) != 0 || stats(t, r.st, r.now)[collect.OCRBusy].N != 1 {
		t.Fatalf("calls %v stats %+v", r.cap.calls, stats(t, r.st, r.now))
	}
}

func TestOCRTestRequest(t *testing.T) {
	r := newOCRRig(t)
	r.cfg.OCR.Enabled = false // a test read works with reading off
	r.open(1, "term", "zsh")
	s := span.Sample{BundleID: "term", App: "term", Title: strPtr("zsh")}
	if err := r.st.SetState(keyOCRRequest, "test "+itoa(r.now+3_000)+" n1", r.now); err != nil {
		t.Fatal(err)
	}
	r.sample(2_000, s)
	if len(r.cap.calls) != 0 {
		t.Fatal("read before not_before")
	}
	r.sample(2_000, s)
	if res, _, _ := r.st.State(keyOCRResult); res != "n1 term\tok\t140\tkubectl rollout" {
		t.Fatalf("result = %q", res)
	}
	if words(t, r.st, 1) != nil {
		t.Fatal("a test read was stored")
	}
	if req, _, _ := r.st.State(keyOCRRequest); req != "" {
		t.Fatalf("request not cleared: %q", req)
	}

	// Privacy checks still apply.
	if err := r.st.SetState(keyOCRRequest, "test 0 n2", r.now); err != nil {
		t.Fatal(err)
	}
	r.sample(2_000, span.Sample{BundleID: "com.apple.Safari", App: "Safari", Private: true})
	if res, _, _ := r.st.State(keyOCRResult); res != "n2 Safari\tprivate\t0\t" || len(r.cap.calls) != 1 {
		t.Fatalf("private result = %q, calls %v", res, r.cap.calls)
	}

	// Grant.
	r.cap.granted = true
	if err := r.st.SetState(keyOCRRequest, "grant n3", r.now); err != nil {
		t.Fatal(err)
	}
	r.sample(2_000, s)
	if res, _, _ := r.st.State(keyOCRResult); res != "n3 grant\tgranted" {
		t.Fatalf("grant result = %q", res)
	}
}

func TestOCRReopenAndDelete(t *testing.T) {
	r := newOCRRig(t)
	r.open(1, "term", "zsh")
	r.sample(5_000, span.Sample{BundleID: "term", App: "term", Title: strPtr("zsh")})
	r.c.observe([]span.Op{{Kind: span.OpClose, ID: 1, TS: r.now}}, nil)
	if r.c.cur.ID != 0 {
		t.Fatal("closed span still current")
	}
	r.c.observe([]span.Op{{Kind: span.OpReopen, ID: 1}}, nil)
	if r.c.cur.ID != 1 || r.c.cur.Start != ocrT0 {
		t.Fatalf("reopen = %+v", r.c.cur)
	}
	r.sample(16_000, span.Sample{BundleID: "term", App: "term", Title: strPtr("zsh")})
	if len(r.cap.calls) != 1 {
		t.Fatal("reopened span not read after its dwell")
	}
}
func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestRunOCRWorkerArgs(t *testing.T) {
	var out bytes.Buffer
	for _, args := range [][]string{nil, {"--bundle"}, {"--bundle", ""}, {"--nope"}} {
		if rc := runOCRWorker(args, &out); rc != collect.OCRExit(collect.OCRError) {
			t.Errorf("%q: rc = %d", args, rc)
		}
	}
	if out.Len() != 0 {
		t.Fatalf("printed %q", out.String())
	}
}
