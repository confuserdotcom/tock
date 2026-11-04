package main

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"tock/internal/config"
	"tock/internal/store"
)

// fakeTrackd answers the pending OCR request on the first Sleep, as trackd
// would, with body after the nonce.
func fakeTrackd(t *testing.T, te *tenv, body string) *string {
	t.Helper()
	var req string
	te.Sleep = func(d time.Duration) {
		te.now = te.now.Add(d)
		st, err := store.Open(te.dbPath())
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		v, _, _ := st.State(store.KeyOCRRequest)
		if v == "" {
			return
		}
		req = v
		f := strings.Fields(v)
		if err := st.SetState(store.KeyOCRResult, f[len(f)-1]+" "+body, te.now.UnixMilli()); err != nil {
			t.Fatal(err)
		}
		if err := st.SetState(store.KeyOCRRequest, "", te.now.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	return &req
}

func TestOCRTestCommand(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	req := fakeTrackd(t, te, "Ghostty\tok\t142\tkubectl deployment rollout")
	code, out := te.run(t, "", "ocr", "test")
	if code != 0 || out != "capturing the front window in 3 s (switch to it now)...\nGhostty · keywords: kubectl deployment rollout · 142 ms\n" {
		t.Fatalf("code %d out %q err %q", code, out, te.errb.String())
	}
	if f := strings.Fields(*req); len(f) != 3 || f[0] != "test" || f[1] != strconv.FormatInt(testNow.UnixMilli()+3000, 10) {
		t.Fatalf("request = %q", *req)
	}

	fakeTrackd(t, te, "Safari\tprivate\t0\t")
	if code, out := te.run(t, "", "ocr", "test", "--now"); code != 0 || out != "Safari · skipped: private window\n" {
		t.Fatalf("skip: code %d out %q", code, out)
	}

	fakeTrackd(t, te, "grant\tmissing")
	if code, out := te.run(t, "", "ocr", "grant"); code != 0 || !strings.Contains(out, "not granted yet") {
		t.Fatalf("grant: code %d out %q", code, out)
	}

	te.Sleep = func(d time.Duration) { te.now = te.now.Add(d) } // trackd never answers
	if code, _ := te.run(t, "", "ocr", "test", "--now"); code != 1 || !strings.Contains(te.errb.String(), "trackd did not answer") {
		t.Fatalf("no trackd: code %d err %q", code, te.errb.String())
	}
	if code, _ := te.run(t, "", "ocr", "nope"); code != 2 {
		t.Fatalf("usage: code %d", code)
	}
}

func TestOCRDoctorLine(t *testing.T) {
	on := config.DefaultOCR()
	on.Enabled = true
	for _, c := range []struct {
		name      string
		cfg       config.OCR
		linux     bool
		perm      string
		stats     map[string]store.OCRStat
		want, fix string
	}{
		{"linux", on, true, "", nil, "unavailable on Linux", ""},
		{"off", config.DefaultOCR(), false, "", nil, "off", ""},
		{"missing", on, false, "missing", nil, "on · permission missing", "tock ocr grant"},
		{"unknown", on, false, "", nil, "on · permission unknown until the first read · today 0 reads", ""},
		{"granted", on, false, "granted", map[string]store.OCRStat{"ok": {N: 37, CostMS: 37 * 138}, "private": {N: 4}, "afk": {N: 12}},
			"on · permission granted · today 37 reads, avg 138 ms · skipped: away 12, private window 4", ""},
	} {
		got, fix := ocrDoctor(c.cfg, c.linux, c.perm, c.stats)
		if got != c.want || fix != c.fix {
			t.Errorf("%s: %q, %q; want %q, %q", c.name, got, fix, c.want, c.fix)
		}
	}
}
