package collect

import (
	"testing"

	"tock/internal/span"
)

// fakeRunner records script calls and returns queued results.
type fakeRunner struct {
	calls   int
	warm    []bool
	results []fakeResult
}

type fakeResult struct {
	out    string
	errnum int
	ok     bool
}

func (f *fakeRunner) run(kind browserKind, warm bool) (string, int, bool) {
	f.calls++
	f.warm = append(f.warm, warm)
	if len(f.results) == 0 {
		return "", aeErrTimeout, false
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r.out, r.errnum, r.ok
}

func okURL(u string) fakeResult  { return fakeResult{out: u, ok: true} }
func fail(errnum int) fakeResult { return fakeResult{errnum: errnum} }
func strp(s string) *string      { return &s }
func deref(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func sample(bundle string, title *string) *span.Sample {
	return &span.Sample{BundleID: bundle, App: "x", Title: title, AXTrusted: true}
}

const (
	chromeID  = "com.google.Chrome"
	arcID     = "company.thebrowser.Browser"
	safariID  = "com.apple.Safari"
	firefoxID = "org.mozilla.firefox"
	t0        = int64(1700000000000)
)

func TestBrowserForBundle(t *testing.T) {
	want := map[string]browserKind{
		safariID:                   browserSafari,
		chromeID:                   browserChrome,
		arcID:                      browserArc,
		"com.brave.Browser":        browserBrave,
		firefoxID:                  browserFirefox,
		"com.apple.Terminal":       browserNone,
		"":                         browserNone,
		"com.google.Chrome.canary": browserNone,
	}
	for id, k := range want {
		if got := browserForBundle(id); got != k {
			t.Errorf("browserForBundle(%q) = %d; want %d", id, got, k)
		}
	}
}

func TestPrivateSafariTitleSuffix(t *testing.T) {
	if !isSafariPrivateTitle("Example Domain, Private Browsing") {
		t.Error("Safari private suffix not detected")
	}
	if isSafariPrivateTitle("Private Browsing notes") {
		t.Error("Safari title starting with Private Browsing treated as private")
	}
}

func TestPrivateFirefoxTitleSuffix(t *testing.T) {
	for _, title := range []string{
		"Example — Private Browsing",
		"Example - Private Browsing",
		"Example — Private Browsing  ",
	} {
		if !isFirefoxPrivateTitle(title) {
			t.Errorf("isFirefoxPrivateTitle(%q) = false; want true", title)
		}
	}
	if isFirefoxPrivateTitle("Private Browsing guide - Mozilla") {
		t.Error("non-private Firefox title treated as private")
	}
}

func TestPrivateApplyBlanksTitleAndURL(t *testing.T) {
	s := sample(chromeID, strp("Secret"))
	s.URL = strp("secret.example/x")
	applyPrivacy(s)
	if !s.Private || s.Title != nil || s.URL != nil {
		t.Fatalf("after applyPrivacy: private=%v title=%s url=%s", s.Private, deref(s.Title), deref(s.URL))
	}
}

func TestPrivateChromiumIncognitoResult(t *testing.T) {
	f := &fakeRunner{results: []fakeResult{okURL("private")}}
	s := sample(chromeID, strp("Secret page"))
	applyBrowser(s, f.run, newBrowserPolicy(), t0)
	if !s.Private || s.Title != nil || s.URL != nil {
		t.Fatalf("incognito: private=%v title=%s url=%s", s.Private, deref(s.Title), deref(s.URL))
	}
}

func TestPrivateUnknownRedactsChromiumTitle(t *testing.T) {
	f := &fakeRunner{results: []fakeResult{fail(-1728)}}
	s := sample(chromeID, strp("Doc A"))
	applyBrowser(s, f.run, newBrowserPolicy(), t0)
	if s.Title != nil || s.URL != nil || s.Private {
		t.Fatalf("failed script: title=%s url=%s private=%v; want nil, nil, false", deref(s.Title), deref(s.URL), s.Private)
	}
}

func TestPrivateSafariSkipsScript(t *testing.T) {
	f := &fakeRunner{results: []fakeResult{okURL("https://leak.example/")}}
	s := sample(safariID, strp("Example Domain, Private Browsing"))
	applyBrowser(s, f.run, newBrowserPolicy(), t0)
	if f.calls != 0 {
		t.Fatalf("runner calls = %d; want 0", f.calls)
	}
	if !s.Private || s.Title != nil || s.URL != nil {
		t.Fatalf("Safari private: private=%v title=%s url=%s", s.Private, deref(s.Title), deref(s.URL))
	}
}

func TestPrivateSafariNilTitleNoURL(t *testing.T) {
	for name, title := range map[string]*string{"nil": nil, "blank": strp("   ")} {
		f := &fakeRunner{results: []fakeResult{okURL("https://leak.example/")}}
		s := sample(safariID, title)
		s.URL = strp("preset.example/x")
		applyBrowser(s, f.run, newBrowserPolicy(), t0)
		if f.calls != 0 {
			t.Errorf("%s title: runner calls = %d; want 0", name, f.calls)
		}
		if s.Title != nil || s.URL != nil || s.Private {
			t.Errorf("%s title: title=%s url=%s private=%v; want nil, nil, false", name, deref(s.Title), deref(s.URL), s.Private)
		}
	}
}

func TestPrivateSafariAXUntrustedNoScript(t *testing.T) {
	f := &fakeRunner{results: []fakeResult{okURL("https://leak.example/")}}
	s := &span.Sample{BundleID: safariID, App: "Safari", AXTrusted: false}
	applyBrowser(s, f.run, newBrowserPolicy(), t0)
	if f.calls != 0 || s.Title != nil || s.URL != nil {
		t.Fatalf("AX untrusted Safari: calls=%d title=%s url=%s", f.calls, deref(s.Title), deref(s.URL))
	}
}

func TestPrivateSafariNormalKeepsTitleAndURL(t *testing.T) {
	f := &fakeRunner{results: []fakeResult{okURL("https://Example.com/a/?q=1")}}
	s := sample(safariID, strp("Example Domain"))
	applyBrowser(s, f.run, newBrowserPolicy(), t0)
	if deref(s.Title) != "Example Domain" || deref(s.URL) != "example.com/a" || s.Private {
		t.Fatalf("Safari normal: title=%s url=%s private=%v", deref(s.Title), deref(s.URL), s.Private)
	}
}

func TestPrivateLockedNoScript(t *testing.T) {
	f := &fakeRunner{results: []fakeResult{okURL("https://leak.example/")}}
	s := sample(chromeID, strp("Doc A"))
	s.Locked = true
	applyBrowser(s, f.run, newBrowserPolicy(), t0)
	if f.calls != 0 || s.Title != nil || s.URL != nil {
		t.Fatalf("locked Chrome: calls=%d title=%s url=%s", f.calls, deref(s.Title), deref(s.URL))
	}
}

func TestFirefoxNeverScripted(t *testing.T) {
	f := &fakeRunner{results: []fakeResult{okURL("https://leak.example/")}}
	p := newBrowserPolicy()
	normal := sample(firefoxID, strp("Example - Mozilla Firefox"))
	applyBrowser(normal, f.run, p, t0)
	private := sample(firefoxID, strp("Example — Private Browsing"))
	applyBrowser(private, f.run, p, t0+2000)
	if f.calls != 0 {
		t.Fatalf("runner calls = %d; want 0", f.calls)
	}
	if deref(normal.Title) != "Example - Mozilla Firefox" || normal.URL != nil || normal.Automation != "" {
		t.Errorf("normal Firefox: title=%s url=%s automation=%q", deref(normal.Title), deref(normal.URL), normal.Automation)
	}
	if !private.Private || private.Title != nil {
		t.Errorf("private Firefox: private=%v title=%s", private.Private, deref(private.Title))
	}
}

func TestNonBrowserUntouched(t *testing.T) {
	f := &fakeRunner{}
	s := sample("com.apple.Terminal", strp("zsh"))
	applyBrowser(s, f.run, newBrowserPolicy(), t0)
	if f.calls != 0 || deref(s.Title) != "zsh" || s.Automation != "" {
		t.Fatalf("non-browser changed: calls=%d title=%s automation=%q", f.calls, deref(s.Title), s.Automation)
	}
}

// failN records n non-warm failures of errnum starting at now, 1 ms apart.
func failN(p *browserPolicy, kind browserKind, n, errnum int, now int64) {
	for i := 0; i < n; i++ {
		p.record(kind, false, errnum, false, now+int64(i))
	}
}

func TestBackoffAfterThreeFailures(t *testing.T) {
	p := newBrowserPolicy()
	p.record(browserChrome, true, 0, true, t0) // warmed
	failN(p, browserChrome, 3, -1728, t0)
	last := t0 + 2
	if run, _ := p.plan(browserChrome, last+29999); run {
		t.Error("plan at +29999 ms: run=true; want false")
	}
	if run, _ := p.plan(browserChrome, last+30000); !run {
		t.Error("plan at +30000 ms: run=false; want true")
	}
}

func TestBackoffProbationSingleFailureBacksOffAgain(t *testing.T) {
	p := newBrowserPolicy()
	p.record(browserChrome, true, 0, true, t0)
	failN(p, browserChrome, 3, -1728, t0)
	retry := t0 + 2 + 30000
	if run, _ := p.plan(browserChrome, retry); !run {
		t.Fatal("probation retry not allowed")
	}
	p.record(browserChrome, false, -1728, false, retry)
	if run, _ := p.plan(browserChrome, retry+29999); run {
		t.Error("after probation failure: run=true at +29999 ms; want false")
	}
	if run, _ := p.plan(browserChrome, retry+30000); !run {
		t.Error("after probation failure: run=false at +30000 ms; want true")
	}
}

func TestBackoffSuccessResetsCount(t *testing.T) {
	p := newBrowserPolicy()
	p.record(browserChrome, true, 0, true, t0)
	failN(p, browserChrome, 2, -1728, t0)
	p.record(browserChrome, true, 0, false, t0+10)
	failN(p, browserChrome, 2, -1728, t0+20)
	if run, _ := p.plan(browserChrome, t0+30); !run {
		t.Fatal("2+success+2 failures started a backoff; success must reset the count")
	}
}

func TestBackoffColdFirstTimeoutNotCounted(t *testing.T) {
	p := newBrowserPolicy()
	run, warm := p.plan(browserArc, t0)
	if !run || !warm {
		t.Fatalf("first plan = %v, %v; want run, warm", run, warm)
	}
	p.record(browserArc, false, aeErrTimeout, true, t0)
	failN(p, browserArc, 2, aeErrTimeout, t0+1)
	run, warm = p.plan(browserArc, t0+10)
	if !run {
		t.Fatal("cold warm-up timeout was counted: backoff after 1+2 failures")
	}
	if warm {
		t.Fatal("second plan still warm; want warm=false after the warm-up call")
	}
}

func TestBackoffDeniedMarksAutomationDenied(t *testing.T) {
	p := newBrowserPolicy()
	p.record(browserChrome, false, aeErrNotPermitted, true, t0)
	if got := p.automation(browserChrome); got != span.AutomationDenied {
		t.Fatalf("automation = %q; want denied", got)
	}
}

func TestAutomationStatusMapping(t *testing.T) {
	want := map[int]string{
		0:     span.AutomationGranted,
		-1743: span.AutomationDenied,
		-1744: span.AutomationUnknown,
		-600:  span.AutomationNotRunning,
		42:    span.AutomationUnknown,
	}
	for code, w := range want {
		if got := automationStatus(code); got != w {
			t.Errorf("automationStatus(%d) = %q; want %q", code, got, w)
		}
	}
}

func TestAutomationCheckedOncePerBrowser(t *testing.T) {
	p := newBrowserPolicy()
	if !p.needsAutomationCheck(browserChrome) {
		t.Fatal("first check for Chrome: false; want true")
	}
	if p.needsAutomationCheck(browserChrome) {
		t.Fatal("second check for Chrome: true; want false")
	}
	if !p.needsAutomationCheck(browserSafari) {
		t.Fatal("first check for Safari: false; want true")
	}
}

func TestAutomationAttachedToSample(t *testing.T) {
	p := newBrowserPolicy()
	p.setAutomation(browserChrome, span.AutomationGranted)
	s := sample(chromeID, strp("Doc"))
	applyBrowser(s, (&fakeRunner{results: []fakeResult{okURL("https://e.com/")}}).run, p, t0)
	if s.Automation != span.AutomationGranted {
		t.Fatalf("Automation = %q; want granted", s.Automation)
	}
	s = sample(chromeID, strp("Doc"))
	applyBrowser(s, (&fakeRunner{results: []fakeResult{fail(aeErrNotPermitted)}}).run, p, t0+2000)
	if s.Automation != span.AutomationDenied {
		t.Fatalf("Automation after -1743 = %q; want denied", s.Automation)
	}
}

// confirmDocA records a successful Chrome poll for title "Doc A" at t0.
func confirmDocA(t *testing.T, p *browserPolicy) {
	t.Helper()
	s := sample(chromeID, strp("Doc A"))
	applyBrowser(s, (&fakeRunner{results: []fakeResult{okURL("https://example.com/a")}}).run, p, t0)
	if deref(s.URL) != "example.com/a" {
		t.Fatalf("confirming poll url = %s", deref(s.URL))
	}
}

func TestVerdictReusedWithinTenSecondsSameTitle(t *testing.T) {
	p := newBrowserPolicy()
	confirmDocA(t, p)
	s := sample(chromeID, strp("Doc A"))
	applyBrowser(s, (&fakeRunner{results: []fakeResult{fail(aeErrTimeout)}}).run, p, t0+9999)
	if deref(s.Title) != "Doc A" || deref(s.URL) != "example.com/a" {
		t.Fatalf("failure within TTL: title=%s url=%s; want Doc A, example.com/a", deref(s.Title), deref(s.URL))
	}

	// The same holds when the poll is a backoff skip.
	p = newBrowserPolicy()
	confirmDocA(t, p)
	p.mu.Lock()
	p.state(browserChrome).until = t0 + backoffMS
	p.mu.Unlock()
	f := &fakeRunner{}
	s = sample(chromeID, strp("Doc A"))
	applyBrowser(s, f.run, p, t0+9999)
	if f.calls != 0 {
		t.Fatalf("backoff skip ran the script")
	}
	if deref(s.Title) != "Doc A" || deref(s.URL) != "example.com/a" {
		t.Fatalf("backoff skip within TTL: title=%s url=%s", deref(s.Title), deref(s.URL))
	}
}

func TestVerdictExpiresAfterTenSeconds(t *testing.T) {
	p := newBrowserPolicy()
	confirmDocA(t, p)
	s := sample(chromeID, strp("Doc A"))
	applyBrowser(s, (&fakeRunner{results: []fakeResult{fail(aeErrTimeout)}}).run, p, t0+10001)
	if s.Title != nil || s.URL != nil {
		t.Fatalf("failure after TTL: title=%s url=%s; want nil, nil", deref(s.Title), deref(s.URL))
	}
}

func TestVerdictNotReusedAcrossTitleChange(t *testing.T) {
	for name, title := range map[string]*string{"Doc B": strp("Doc B"), "nil": nil} {
		p := newBrowserPolicy()
		confirmDocA(t, p)
		s := sample(chromeID, title)
		applyBrowser(s, (&fakeRunner{results: []fakeResult{fail(aeErrTimeout)}}).run, p, t0+2000)
		if s.Title != nil || s.URL != nil {
			t.Errorf("title %s: title=%s url=%s; want nil, nil", name, deref(s.Title), deref(s.URL))
		}
	}
}

func TestVerdictClearedByIncognitoAndDenied(t *testing.T) {
	p := newBrowserPolicy()
	confirmDocA(t, p)
	inc := sample(chromeID, strp("Doc A"))
	applyBrowser(inc, (&fakeRunner{results: []fakeResult{okURL("private")}}).run, p, t0+2000)
	s := sample(chromeID, strp("Doc A"))
	applyBrowser(s, (&fakeRunner{results: []fakeResult{fail(aeErrTimeout)}}).run, p, t0+4000)
	if s.Title != nil || s.URL != nil {
		t.Fatalf("after incognito: title=%s url=%s; want nil, nil", deref(s.Title), deref(s.URL))
	}

	p = newBrowserPolicy()
	confirmDocA(t, p)
	s = sample(chromeID, strp("Doc A"))
	applyBrowser(s, (&fakeRunner{results: []fakeResult{fail(aeErrNotPermitted)}}).run, p, t0+2000)
	if s.Title != nil || s.URL != nil {
		t.Fatalf("-1743 within TTL: title=%s url=%s; want nil, nil", deref(s.Title), deref(s.URL))
	}
}

func TestVerdictClearedByLock(t *testing.T) {
	p := newBrowserPolicy()
	confirmDocA(t, p)
	locked := sample(chromeID, strp("Doc A"))
	locked.Locked = true
	applyBrowser(locked, (&fakeRunner{}).run, p, t0+1000)
	s := sample(chromeID, strp("Doc A"))
	applyBrowser(s, (&fakeRunner{results: []fakeResult{fail(aeErrTimeout)}}).run, p, t0+2000)
	if s.Title != nil || s.URL != nil {
		t.Fatalf("after lock: title=%s url=%s; want nil, nil", deref(s.Title), deref(s.URL))
	}
}
