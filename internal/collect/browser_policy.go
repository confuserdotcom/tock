package collect

import (
	"strings"
	"sync"

	"tock/internal/span"
)

// browserKind identifies a supported browser. The scriptable kinds share
// their numbers with the TOCK_SCRIPT_* kinds in browser.h.
type browserKind int

const (
	browserNone    browserKind = 0
	browserChrome  browserKind = 1
	browserArc     browserKind = 2
	browserBrave   browserKind = 3
	browserSafari  browserKind = 4
	browserFirefox browserKind = 5
)

// AppleScript error numbers the policy distinguishes.
const (
	aeErrTimeout      = -1712 // event timed out (expected on a cold first call)
	aeErrNotPermitted = -1743 // Automation denied by the user
	aeErrProcNotFound = -600
)

const (
	// backoffMS is how long a browser is skipped after repeated failures.
	backoffMS = 30000
	// backoffAfter is the number of consecutive failures that starts a backoff.
	backoffAfter = 3
	// verdictTTLMS bounds how long a confirmed non-incognito verdict may be
	// reused after a transient failure. It equals the D-16 crash-loss bound.
	verdictTTLMS = 10000
)

// browserForBundle maps a bundle id to its browser kind.
func browserForBundle(bundleID string) browserKind {
	switch bundleID {
	case "com.google.Chrome":
		return browserChrome
	case "company.thebrowser.Browser":
		return browserArc
	case "com.brave.Browser":
		return browserBrave
	case "com.apple.Safari":
		return browserSafari
	case "org.mozilla.firefox":
		return browserFirefox
	}
	return browserNone
}

// isSafariPrivateTitle reports whether a Safari AX window title marks a
// private window (English UI only, RESEARCH Q6).
func isSafariPrivateTitle(t string) bool {
	return strings.HasSuffix(t, ", Private Browsing")
}

// isFirefoxPrivateTitle reports whether a Firefox window title marks a
// private window ("… — Private Browsing" or "… - Private Browsing").
func isFirefoxPrivateTitle(t string) bool {
	return strings.HasSuffix(strings.TrimSpace(t), "Private Browsing")
}

// applyPrivacy marks a sample private and drops its title and URL (D-07).
func applyPrivacy(s *span.Sample) {
	s.Private = true
	s.Title = nil
	s.URL = nil
}

// scriptRunner runs the front-tab script for a browser. ok=false carries the
// AppleScript error number.
type scriptRunner func(kind browserKind, warm bool) (out string, errnum int, ok bool)

type verdict struct {
	title string
	url   *string
	at    int64
	ok    bool
}

type browserState struct {
	fails      int
	until      int64
	probation  bool
	warmed     bool
	checked    bool
	automation string
	verdict    verdict
}

// browserPolicy holds the per-browser failure backoff (D-21), Automation
// status (D-20) and the privacy-verdict cache. It is shared between the main
// thread and the Automation-check callback, so it is locked.
type browserPolicy struct {
	mu sync.Mutex
	st map[browserKind]*browserState
}

func newBrowserPolicy() *browserPolicy {
	return &browserPolicy{st: make(map[browserKind]*browserState)}
}

// state returns the state for kind; p.mu must be held.
func (p *browserPolicy) state(kind browserKind) *browserState {
	st, ok := p.st[kind]
	if !ok {
		st = &browserState{}
		p.st[kind] = st
	}
	return st
}

// plan says whether a script may run for kind now, and whether it is the
// first (warm-up) call of the process.
func (p *browserPolicy) plan(kind browserKind, nowMS int64) (run, warm bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.state(kind)
	if nowMS < st.until {
		return false, false
	}
	return true, !st.warmed
}

// record feeds the outcome of a script call. Three consecutive failures, or
// any failure on the probation retry after a backoff, skip the browser for
// backoffMS. A timeout on the warm-up call is not counted.
func (p *browserPolicy) record(kind browserKind, ok bool, errnum int, warm bool, nowMS int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.state(kind)
	if warm {
		st.warmed = true
	}
	if ok {
		st.fails = 0
		st.probation = false
		return
	}
	if errnum == aeErrNotPermitted {
		st.automation = span.AutomationDenied
	}
	if warm && errnum == aeErrTimeout {
		return
	}
	st.fails++
	if st.fails >= backoffAfter || st.probation {
		st.until = nowMS + backoffMS
		st.fails = 0
		st.probation = true
	}
}

// automation returns the Automation status of kind, unknown until known.
func (p *browserPolicy) automation(kind browserKind) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if a := p.state(kind).automation; a != "" {
		return a
	}
	return span.AutomationUnknown
}

// needsAutomationCheck is true exactly once per kind.
func (p *browserPolicy) needsAutomationCheck(kind browserKind) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.state(kind)
	if st.checked {
		return false
	}
	st.checked = true
	return true
}

func (p *browserPolicy) setAutomation(kind browserKind, status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state(kind).automation = status
}

// automationStatus maps an AEDeterminePermissionToAutomateTarget result.
func automationStatus(code int) string {
	switch code {
	case 0:
		return span.AutomationGranted
	case aeErrNotPermitted:
		return span.AutomationDenied
	case aeErrProcNotFound:
		return span.AutomationNotRunning
	}
	return span.AutomationUnknown
}

// rememberVerdict stores a confirmed non-incognito result for the raw window
// title. Only real confirmations refresh the timestamp.
func (p *browserPolicy) rememberVerdict(kind browserKind, title string, url *string, nowMS int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state(kind).verdict = verdict{title: title, url: url, at: nowMS, ok: true}
}

func (p *browserPolicy) clearVerdict(kind browserKind) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state(kind).verdict = verdict{}
}

// reuseVerdict returns the cached URL when the title is byte-identical to the
// confirmed one and the confirmation is at most verdictTTLMS old.
func (p *browserPolicy) reuseVerdict(kind browserKind, title *string, nowMS int64) (*string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v := p.state(kind).verdict
	if !v.ok || title == nil || *title != v.title || nowMS-v.at > verdictTTLMS {
		return nil, false
	}
	return v.url, true
}

// applyBrowser enforces browser privacy and fills the URL before a sample
// leaves the collector. A window whose privacy is not established never gets
// a URL, and a Chromium window never keeps its title (over-redact).
func applyBrowser(s *span.Sample, run scriptRunner, p *browserPolicy, nowMS int64) {
	kind := browserForBundle(s.BundleID)
	switch kind {
	case browserNone:
		return
	case browserFirefox:
		// Title-only: no script is ever sent to Firefox.
		s.URL = nil
		if s.Title != nil && isFirefoxPrivateTitle(*s.Title) {
			applyPrivacy(s)
		}
		return
	}
	defer func() { s.Automation = p.automation(kind) }()

	if kind == browserSafari {
		// Safari privacy is only knowable from the AX title (D-07): no usable
		// title means privacy-unknown, so no script, no title and no URL.
		if s.Title == nil || strings.TrimSpace(*s.Title) == "" {
			s.Title = nil
			s.URL = nil
			return
		}
		if isSafariPrivateTitle(*s.Title) {
			applyPrivacy(s)
			return
		}
	}
	if s.Locked {
		p.clearVerdict(kind)
		s.URL = nil
		if kind != browserSafari {
			s.Title = nil
		}
		return
	}

	errnum := 0
	if ok, warm := p.plan(kind, nowMS); ok {
		out, n, ok := run(kind, warm)
		p.record(kind, ok, n, warm, nowMS)
		if ok {
			if kind != browserSafari && out == "private" {
				p.clearVerdict(kind)
				applyPrivacy(s)
				return
			}
			s.URL = nil
			if key, ok := span.URLKey(out); ok {
				s.URL = &key
			}
			if s.Title != nil {
				p.rememberVerdict(kind, *s.Title, s.URL, nowMS)
			}
			return
		}
		errnum = n
	}

	// Transient failure or backoff skip: reuse a fresh verdict for the same
	// window title so one slow poll does not split the span. Denial never
	// reuses.
	if errnum == aeErrNotPermitted {
		p.clearVerdict(kind)
	} else if url, ok := p.reuseVerdict(kind, s.Title, nowMS); ok {
		s.URL = url
		return
	}
	s.URL = nil
	if kind != browserSafari {
		s.Title = nil
	}
}
