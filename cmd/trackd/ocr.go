package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"tock/internal/classify"
	"tock/internal/collect"
	"tock/internal/config"
	"tock/internal/span"
	"tock/internal/store"
)

// daemon_state keys of screen reading: tock asks, trackd answers.
const (
	keyOCRRequest = store.KeyOCRRequest
	keyOCRResult  = store.KeyOCRResult
	keyOCRPerm    = store.KeyOCRPerm
)

const (
	ocrReloadMS   = 60_000 // config and rules are read again after this
	ocrPermWaitMS = 60_000 // a missing permission is not asked again before this
)

// secureTitle matches sign-in and password windows: never read.
var secureTitle = regexp.MustCompile(`(?i)\b(sign[ -]?in|log[ -]?in|password|passcode|passwort|mot de passe|connexion|authenticat\w*|2fa|two[- ]factor|otp|keychain|verification code)\b`)

// ocrStore is what the coordinator needs from the store.
type ocrStore interface {
	SpanWords(ids []int64) (map[int64][]string, error)
	SetSpanWords(id int64, words []string, atMS int64) error
	CountOCR(day, reason string, costMS int64) error
	State(key string) (string, bool, error)
	SetState(key, value string, nowMS int64) error
}

// ocrGranter asks for the Screen Recording permission.
type ocrGranter interface {
	Grant(ctx context.Context) (bool, error)
}

// ocrSettings are the parts of config.toml and the rules screen reading
// uses, compiled.
type ocrSettings struct {
	cfg                   config.OCR
	apps, deny            map[string]bool
	denyHosts, denyTitles []*regexp.Regexp
	rs                    *classify.Ruleset // nil: every window counts as unclear
}

// newOCRSettings compiles c's [ocr] and [llm] deny lists.
func newOCRSettings(c config.Config, rs *classify.Ruleset) (ocrSettings, error) {
	s := ocrSettings{cfg: c.OCR, apps: map[string]bool{}, deny: map[string]bool{}, rs: rs}
	for _, b := range c.OCR.Apps {
		s.apps[b] = true
	}
	for _, b := range append(append([]string{}, c.LLM.DenyApps...), c.OCR.DenyApps...) {
		s.deny[b] = true
	}
	for _, p := range append(append([]string{}, c.LLM.DenyHosts...), c.OCR.DenyHosts...) {
		re, err := regexp.Compile(p)
		if err != nil {
			return s, fmt.Errorf("ocr: deny host %q: %w", p, err)
		}
		s.denyHosts = append(s.denyHosts, re)
	}
	for _, p := range append(append([]string{}, c.LLM.DenyTitles...), c.OCR.DenyTitles...) {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return s, fmt.Errorf("ocr: deny title %q: %w", p, err)
		}
		s.denyTitles = append(s.denyTitles, re)
	}
	return s, nil
}

// ocrJob is one read the worker goroutine runs.
type ocrJob struct {
	spanID int64 // 0 for a test read
	bundle string
	app    string // the app's name, shown by a test read
	nonce  string // a tock ocr test or grant request; "" otherwise
	grant  bool
	deny   []*regexp.Regexp // deny title patterns, also applied to words
}

// ocrCoord decides when the front window is read: cheap checks
// on every sample, then at most one worker at a time. Reads run off the
// collector's path; their keywords merge into the span's bag.
type ocrCoord struct {
	cap     collect.Capturer // nil: screen reading is unavailable here
	secure  func() bool      // secure event input
	load    func() (ocrSettings, error)
	st      ocrStore
	log     *slog.Logger
	now     func() int64
	idleMS  int64 // the idle threshold: no input for this long is away
	runSync bool  // tests: jobs run inline

	set      ocrSettings
	loadedMS int64
	loaded   bool

	cur      span.Span           // the open span; ID 0 = none
	lastRead int64               // when the open span was last read
	recent   map[int64]span.Span // the last spans opened, for a reopen

	mu        sync.Mutex
	counted   string // the skip already counted for the open span
	busy      bool
	permUntil int64 // no_permission is assumed until then
	jobs      chan ocrJob
	done      chan struct{} // closed when the worker goroutine ends
}

func newOCRCoord(c *ocrCoord) *ocrCoord {
	if c.secure == nil {
		c.secure = func() bool { return false }
	}
	if !c.runSync {
		c.jobs = make(chan ocrJob, 1)
		c.done = make(chan struct{})
		go func() {
			defer close(c.done)
			for j := range c.jobs {
				c.run(j)
			}
		}()
	}
	return c
}

// close stops the worker goroutine once a read in progress (at most
// collect.OCRTimeout) is done, so nothing writes to a closed store.
func (c *ocrCoord) close() {
	if c.jobs != nil {
		close(c.jobs)
		<-c.done
	}
}

// observe follows the merger's ops to know the open span, then judges the
// sample.
func (c *ocrCoord) observe(ops []span.Op, s *span.Sample) {
	for _, op := range ops {
		switch op.Kind {
		case span.OpOpen:
			c.cur, c.lastRead = op.Span, 0
			c.resetCounted()
			c.remember(op.Span)
		case span.OpReopen:
			if op.ID != c.cur.ID {
				sp, ok := c.recent[op.ID]
				if !ok { // unknown start: the dwell starts now
					sp = span.Span{ID: op.ID, Start: c.now()}
				}
				c.cur, c.lastRead = sp, 0
				c.resetCounted()
			}
		case span.OpClose, span.OpDelete:
			if op.ID == c.cur.ID {
				c.cur = span.Span{}
			}
		}
	}
	if s == nil {
		return
	}
	now := c.now()
	if !c.loaded || now-c.loadedMS >= ocrReloadMS {
		c.loadedMS, c.loaded = now, true
		set, err := c.load()
		if err != nil {
			c.log.Warn("ocr settings", "err", err) // the previous ones stay
		} else {
			c.set = set
		}
	}
	if c.request(s, now) {
		return
	}
	reason := c.decide(s, now)
	switch reason {
	case "":
		c.start(ocrJob{spanID: c.cur.ID, bundle: s.BundleID, deny: c.set.denyTitles}, now)
	case collect.OCROff, collect.OCRDwell, collect.OCRThrottle:
		// waiting is normal: not counted
	default:
		c.count(reason, 0, now)
	}
}

// decide is the skip reason of reading s now, "" to read. The order is
// locked: cheap checks first, nothing is captured for a skipped window.
func (c *ocrCoord) decide(s *span.Sample, now int64) string {
	if !c.set.cfg.Enabled {
		return collect.OCROff
	}
	if c.cap == nil {
		return collect.OCRUnavailable
	}
	if c.cur.ID == 0 {
		return collect.OCRDwell
	}
	c.mu.Lock()
	perm := now < c.permUntil
	c.mu.Unlock()
	if perm {
		return collect.OCRNoPermission
	}
	if c.cur.BundleID == span.IdleBundleID || s.IdleS*1000 >= float64(c.idleMS) {
		return collect.OCRAway
	}
	if r := c.guard(s); r != "" {
		return r
	}
	if !c.set.apps[s.BundleID] && c.set.rs != nil {
		v := classify.SpanView{BundleID: s.BundleID, App: s.App, Title: s.Title, URL: s.URL}
		if !c.set.rs.Unclear(c.set.rs.Label(v, nil)) {
			return collect.OCROff // labelled well enough: nothing to read
		}
	}
	if now-c.cur.Start < c.set.cfg.MinDwellMS {
		return collect.OCRDwell
	}
	if c.lastRead > 0 && now-c.lastRead < c.set.cfg.IntervalMS {
		return collect.OCRThrottle
	}
	return ""
}

// guard is the privacy checks a test read also passes: locked, secure,
// deny, private.
func (c *ocrCoord) guard(s *span.Sample) string {
	if s.Locked || s.BundleID == span.LoginWindowBundleID {
		return collect.OCRLocked
	}
	title, host := "", ""
	if s.Title != nil {
		title = *s.Title
	}
	if s.URL != nil {
		host = store.URLHost(*s.URL)
	}
	if c.secure() || secureTitle.MatchString(title) || secureTitle.MatchString(host) {
		return collect.OCRSecure
	}
	if c.set.deny[s.BundleID] {
		return collect.OCRDeny
	}
	for _, re := range c.set.denyHosts {
		if host != "" && re.MatchString(host) {
			return collect.OCRDeny
		}
	}
	for _, re := range c.set.denyTitles {
		if title != "" && re.MatchString(title) {
			return collect.OCRDeny
		}
	}
	// A browser whose window state is unknown may be private: fail closed.
	if s.Private || (collect.IsBrowser(s.BundleID) && s.Title == nil) {
		return collect.OCRPrivate
	}
	return ""
}

// request runs a tock ocr test or grant request when one is due and
// reports whether it did.
func (c *ocrCoord) request(s *span.Sample, now int64) bool {
	v, ok, err := c.st.State(keyOCRRequest)
	if err != nil || !ok || v == "" {
		return false
	}
	f := strings.Fields(v)
	switch {
	case len(f) == 2 && f[0] == "grant":
		c.clearRequest(now)
		if c.cap == nil {
			c.answer(f[1], "grant\t"+collect.OCRUnavailable, now)
			return true
		}
		c.start(ocrJob{nonce: f[1], grant: true}, now)
		return true
	case len(f) == 3 && f[0] == "test":
		at, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			c.clearRequest(now)
			return false
		}
		if now < at {
			return false // the user is switching to the window
		}
		c.clearRequest(now)
		if c.cap == nil {
			c.answer(f[2], s.App+"\t"+collect.OCRUnavailable+"\t0\t", now)
			return true
		}
		if r := c.guard(s); r != "" {
			c.answer(f[2], s.App+"\t"+r+"\t0\t", now)
			return true
		}
		c.start(ocrJob{nonce: f[2], bundle: s.BundleID, app: s.App, deny: c.set.denyTitles}, now)
		return true
	}
	c.clearRequest(now)
	return false
}

func (c *ocrCoord) clearRequest(now int64) {
	if err := c.st.SetState(keyOCRRequest, "", now); err != nil {
		c.log.Warn("ocr request", "err", err)
	}
}

func (c *ocrCoord) answer(nonce, body string, now int64) {
	if err := c.st.SetState(keyOCRResult, nonce+" "+body, now); err != nil {
		c.log.Warn("ocr answer", "err", err)
	}
}

// start hands j to the worker, or counts busy when one runs.
func (c *ocrCoord) start(j ocrJob, now int64) {
	c.mu.Lock()
	if c.busy {
		c.mu.Unlock()
		if j.nonce != "" {
			c.answer(j.nonce, j.app+"\t"+collect.OCRBusy+"\t0\t", now)
		} else {
			c.count(collect.OCRBusy, 0, now)
		}
		return
	}
	c.busy = true
	c.mu.Unlock()
	if j.nonce == "" {
		c.lastRead = now
	}
	if c.runSync {
		c.run(j)
		return
	}
	c.jobs <- j // never blocks: busy guards the one slot
}

// run is one job on the worker goroutine.
func (c *ocrCoord) run(j ocrJob) {
	defer func() {
		c.mu.Lock()
		c.busy = false
		c.mu.Unlock()
	}()
	if j.grant {
		ok := false
		if g, can := c.cap.(ocrGranter); can {
			var err error
			if ok, err = g.Grant(context.Background()); err != nil {
				c.log.Warn("ocr grant", "err", err)
			}
		}
		state := "missing"
		if ok {
			state = "granted"
		}
		c.perm(ok, c.now())
		c.answer(j.nonce, "grant\t"+state, c.now())
		return
	}
	r := c.cap.Capture(context.Background(), j.bundle)
	now := c.now()
	switch r.Reason {
	case collect.OCROK:
		c.perm(true, now)
	case collect.OCRNoPermission:
		c.perm(false, now)
	}
	words := classify.Keywords(r.Lines, j.deny)
	if j.nonce != "" { // a test read is shown, never stored
		c.answer(j.nonce, fmt.Sprintf("%s\t%s\t%d\t%s", j.app, r.Reason, r.CostMS, strings.Join(words, " ")), now)
		return
	}
	if r.Reason != collect.OCROK {
		c.log.Info("ocr skipped", "reason", r.Reason) // never any text
		c.count(r.Reason, 0, now)
		return
	}
	c.count(collect.OCROK, r.CostMS, now)
	if len(words) == 0 {
		return
	}
	old, err := c.st.SpanWords([]int64{j.spanID})
	if err == nil {
		err = c.st.SetSpanWords(j.spanID, classify.MergeWords(old[j.spanID], words), now)
	}
	if err != nil {
		c.log.Warn("ocr words", "err", err)
	}
}

// perm records the permission the worker saw, for tock doctor.
func (c *ocrCoord) perm(granted bool, now int64) {
	c.mu.Lock()
	if granted {
		c.permUntil = 0
	} else {
		c.permUntil = now + ocrPermWaitMS
	}
	c.mu.Unlock()
	v := "missing"
	if granted {
		v = "granted"
	}
	if err := c.st.SetState(keyOCRPerm, v, now); err != nil {
		c.log.Warn("ocr perm", "err", err)
	}
}

// remember keeps the last 8 opened spans.
func (c *ocrCoord) remember(sp span.Span) {
	if c.recent == nil {
		c.recent = map[int64]span.Span{}
	}
	c.recent[sp.ID] = sp
	for len(c.recent) > 8 {
		low := sp.ID
		for id := range c.recent {
			low = min(low, id)
		}
		delete(c.recent, low)
	}
}

func (c *ocrCoord) resetCounted() {
	c.mu.Lock()
	c.counted = ""
	c.mu.Unlock()
}

// count adds one read or skip to today's stats. A skip is counted once per
// span and reason in a row.
func (c *ocrCoord) count(reason string, costMS, now int64) {
	if reason != collect.OCROK {
		c.mu.Lock()
		dup := reason == c.counted
		c.counted = reason
		c.mu.Unlock()
		if dup {
			return
		}
	}
	if err := c.st.CountOCR(dayOf(now), reason, costMS); err != nil {
		c.log.Warn("ocr stats", "err", err)
	}
}

// dayOf is the local day of ms, as ocr_stat keys it.
func dayOf(ms int64) string { return time.UnixMilli(ms).Format("2006-01-02") }

// ocrLoader reads config.toml and the rules for the coordinator. Rules
// that do not load leave every window unclear.
func ocrLoader(configDir string) func() (ocrSettings, error) {
	return func() (ocrSettings, error) {
		c, err := config.Load(filepath.Join(configDir, config.File))
		if err != nil {
			return ocrSettings{}, err
		}
		var rs *classify.Ruleset
		if c.OCR.Enabled { // off: one config read a minute, nothing more
			if rs, err = classify.Load(filepath.Join(configDir, classify.RulesFile), filepath.Join(configDir, classify.LearnedFile)); err != nil {
				rs = nil
			}
		}
		return newOCRSettings(c, rs)
	}
}

// runOCRWorker is "trackd ocr-worker": --bundle B reads the front window of
// B and prints its text; --grant asks for the Screen Recording permission.
// It takes no lock and opens no database. The exit code is the skip reason
// (collect.OCRExit).
func runOCRWorker(args []string, out io.Writer) int {
	switch {
	case len(args) == 1 && args[0] == "--grant":
		if collect.GrantScreen() {
			return 0
		}
		return collect.OCRExit(collect.OCRNoPermission)
	case len(args) == 2 && args[0] == "--bundle" && args[1] != "":
		text, reason := collect.RunOCRWorker(args[1])
		if reason != collect.OCROK {
			return collect.OCRExit(reason)
		}
		if _, err := io.WriteString(out, text); err != nil {
			return collect.OCRExit(collect.OCRError)
		}
		return 0
	}
	return collect.OCRExit(collect.OCRError)
}

// newOCRCapturer is the worker capturer of this binary, nil where screen
// reading is unavailable. Only the binary itself is run, never a PATH
// lookup.
func newOCRCapturer() collect.Capturer {
	if !collect.OCRSupported {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return nil
	}
	return collect.WorkerCapturer{Exe: exe}
}
