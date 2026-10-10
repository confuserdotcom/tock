//go:build darwin && cgo

package collect

/*
#include <stdlib.h>
#include "browser.h"
#include "shim.h"
*/
import "C"

import (
	"strings"
	"time"
	"unsafe"

	"tock/internal/span"
)

// Self-test script kinds (browser.h). They run through the same execution
// path as the browser scripts.
const (
	scriptSelftestURL   browserKind = 90
	scriptSelftestError browserKind = 91
)

// scriptOutLen bounds the copied script result; span.URLKey rejects anything
// longer than 8192 bytes anyway.
const scriptOutLen = 8193

// runScript runs the constant front-tab script for kind. It must be called on
// the main thread or while the main thread pumps its run loop.
func runScript(kind browserKind, warm bool) (string, int, bool) {
	buf := (*C.char)(C.malloc(scriptOutLen))
	defer C.free(unsafe.Pointer(buf))
	w := C.int(0)
	if warm {
		w = 1
	}
	var errnum C.int
	if C.tock_run_script(C.int(kind), w, buf, scriptOutLen, &errnum) != 0 {
		return "", int(errnum), false
	}
	return strings.ToValidUTF8(C.GoString(buf), "�"), 0, true
}

// browsers is the process-wide policy: goAutomationResult has no receiver.
var browsers = newBrowserPolicy()

// bundleForKind is the bundle id whose Automation status is checked.
var bundleForKind = map[browserKind]string{
	browserChrome: "com.google.Chrome",
	browserArc:    "company.thebrowser.Browser",
	browserBrave:  "com.brave.Browser",
	browserSafari: "com.apple.Safari",
}

// applyBrowserDarwin starts the one-time Automation check for a scriptable
// browser, then applies the browser policy with the real script runner.
func (s *darwinSource) applyBrowserDarwin(smp *span.Sample) {
	kind := browserForBundle(smp.BundleID)
	if bundle, ok := bundleForKind[kind]; ok && browsers.needsAutomationCheck(kind) {
		cs := C.CString(bundle)
		ask := C.int(0)
		if s.opts.AskAutomation {
			ask = 1
		}
		C.tock_automation_check(cs, ask)
		C.free(unsafe.Pointer(cs))
	}
	applyBrowser(smp, runScript, browsers, smp.TS)
}

//export goAutomationResult
func goAutomationResult(bundle *C.char, status C.int) {
	kind := browserForBundle(C.GoString(bundle))
	if kind == browserNone {
		return
	}
	browsers.setAutomation(kind, automationStatus(int(status)))
}

// runLoopSlice pumps the main run loop for d. Tests call it from TestMain on
// the main thread so scripts dispatched from test goroutines can run.
func runLoopSlice(d time.Duration) {
	C.tock_run_loop_slice(C.double(d.Seconds()))
}
