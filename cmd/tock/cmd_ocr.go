package main

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"tock/internal/config"
	"tock/internal/store"
)

const (
	ocrTestDelay = 3 * time.Second  // time to switch to the window to read
	ocrWait      = 20 * time.Second // how long tock waits for trackd
	ocrPoll      = 500 * time.Millisecond
)

// ocrReasonText is how a skip reason reads.
var ocrReasonText = map[string]string{
	"off":           "off",
	"no_permission": "no permission",
	"private":       "private window",
	"deny":          "denied by [llm]/[ocr] deny",
	"locked":        "locked screen",
	"afk":           "away",
	"secure":        "sign-in or password window",
	"dwell":         "too soon",
	"throttle":      "too soon",
	"busy":          "busy",
	"changed":       "the front window changed",
	"no_window":     "no window",
	"unavailable":   "unavailable on this machine",
	"error":         "error",
}

func ocrReason(r string) string {
	if t, ok := ocrReasonText[r]; ok {
		return t
	}
	return r
}

// cmdOCR is tock ocr test [--now] and tock ocr grant. tock links no cgo, and
// the Screen Recording grant is trackd's, so trackd does the work: tock
// asks through daemon_state and prints the answer.
func cmdOCR(env Env, args []string) error {
	switch {
	case len(args) >= 1 && args[0] == "test" && (len(args) == 1 || len(args) == 2 && args[1] == "--now"):
		return ocrTest(env, len(args) == 2)
	case len(args) == 1 && args[0] == "grant":
		return ocrGrant(env)
	}
	return usagef("ocr takes test [--now] or grant")
}

func ocrTest(env Env, now bool) error {
	notBefore := env.Now()
	if !now {
		notBefore = notBefore.Add(ocrTestDelay)
		fmt.Fprintf(env.Stdout, "capturing the front window in %d s (switch to it now)...\n", int(ocrTestDelay/time.Second))
	}
	body, err := ocrAsk(env, fmt.Sprintf("test %d", notBefore.UnixMilli()))
	if err != nil {
		return err
	}
	f := strings.SplitN(body, "\t", 4)
	if len(f) != 4 {
		return fmt.Errorf("ocr: unexpected answer from trackd")
	}
	app, reason, cost, words := f[0], f[1], f[2], f[3]
	switch {
	case reason != "ok":
		fmt.Fprintf(env.Stdout, "%s · skipped: %s\n", app, ocrReason(reason))
	case words == "":
		fmt.Fprintf(env.Stdout, "%s · no keywords · %s ms\n", app, cost)
	default:
		fmt.Fprintf(env.Stdout, "%s · keywords: %s · %s ms\n", app, words, cost)
	}
	return nil
}

func ocrGrant(env Env) error {
	body, err := ocrAsk(env, "grant")
	if err != nil {
		return err
	}
	switch strings.TrimPrefix(body, "grant\t") {
	case "granted":
		fmt.Fprintln(env.Stdout, "screen recording: granted to trackd")
	case "unavailable":
		fmt.Fprintln(env.Stdout, "screen recording: unavailable on this machine")
	default:
		fmt.Fprintln(env.Stdout, "screen recording: not granted yet; allow trackd in System Settings › Privacy & Security › Screen & System Audio Recording, then run tock ocr test")
	}
	return nil
}

// ocrAsk writes request (with a fresh nonce) for trackd and waits for its
// answer, which it returns without the nonce.
func ocrAsk(env Env, request string) (string, error) {
	st, err := openStore(env)
	if err != nil {
		return "", err
	}
	defer st.Close()
	nonce := strconv.FormatInt(env.Now().UnixNano(), 36)
	if err := st.SetState(store.KeyOCRRequest, request+" "+nonce, env.Now().UnixMilli()); err != nil {
		return "", err
	}
	deadline := env.Now().Add(ocrWait)
	for {
		v, _, err := st.State(store.KeyOCRResult)
		if err != nil {
			return "", err
		}
		if body, ok := strings.CutPrefix(v, nonce+" "); ok {
			return body, nil
		}
		if !env.Now().Before(deadline) || env.Sleep == nil {
			_ = st.SetState(store.KeyOCRRequest, "", env.Now().UnixMilli())
			return "", errors.New("ocr: trackd did not answer; is it running? (tock doctor)")
		}
		env.Sleep(ocrPoll)
	}
}

// ocrDoctor is the doctor line of screen reading: detail, and a fix when
// it is a problem.
func ocrDoctor(cfg config.OCR, linux bool, perm string, stats map[string]store.OCRStat) (detail, fix string) {
	switch {
	case linux:
		return "unavailable on Linux", ""
	case !cfg.Enabled:
		return "off", ""
	case perm == "missing":
		return "on · permission missing", "tock ocr grant"
	}
	var b strings.Builder
	b.WriteString("on · ")
	if perm == "granted" {
		b.WriteString("permission granted")
	} else {
		b.WriteString("permission unknown until the first read")
	}
	ok := stats["ok"]
	b.WriteString(" · today " + plural(ok.N, "read"))
	if ok.N > 0 {
		fmt.Fprintf(&b, ", avg %d ms", ok.CostMS/int64(ok.N))
	}
	var skips []string
	for _, r := range slices.Sorted(maps.Keys(stats)) {
		if r != "ok" {
			skips = append(skips, fmt.Sprintf("%s %d", ocrReason(r), stats[r].N))
		}
	}
	if len(skips) > 0 {
		b.WriteString(" · skipped: " + strings.Join(skips, ", "))
	}
	return b.String(), ""
}
