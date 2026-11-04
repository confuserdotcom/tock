//go:build darwin && cgo

package collect

/*
#cgo LDFLAGS: -framework ScreenCaptureKit -framework Vision -framework Carbon -framework CoreText -framework CoreGraphics -framework Cocoa
#include <stdlib.h>
#include "ocr_darwin.h"
*/
import "C"

import (
	"strings"
	"unsafe"
)

// OCRSupported reports whether this build can read the screen.
const OCRSupported = true

// RunOCRWorker is the body of "trackd ocr-worker --bundle B": it reads the
// front window of bundle and returns its text, or the skip reason.
func RunOCRWorker(bundle string) (string, string) {
	cb := C.CString(bundle)
	defer C.free(unsafe.Pointer(cb))
	var out *C.char
	rc := int(C.tock_ocr_run(cb, &out))
	return takeText(out), ocrReason(rc)
}

// GrantScreen asks macOS for the Screen Recording permission (a prompt or
// System Settings) and reports whether it is granted. Run it only from
// "trackd ocr-worker --grant", which the user asks for.
func GrantScreen() bool {
	if C.tock_screen_preflight() == 1 {
		return true
	}
	return C.tock_screen_request() == 1
}

// SecureInput reports whether a password field holds the keyboard
// somewhere (secure event input): no screen read then.
func SecureInput() bool { return C.tock_secure_input() == 1 }

// visionText reads text drawn on a white image: Vision without a capture
// or a permission (tests).
func visionText(s string) ([]string, string) {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	var out *C.char
	rc := int(C.tock_ocr_render(cs, &out))
	return strings.Fields(takeText(out)), ocrReason(rc)
}

func takeText(p *C.char) string {
	if p == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}
