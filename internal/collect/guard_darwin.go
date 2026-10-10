//go:build darwin && cgo

package collect

/*
#include <stdlib.h>
#include "browser.h"
*/
import "C"

import "unsafe"

// CloseTab closes the front tab of a scriptable browser when its host is
// still host. Firefox and other apps return false.
func CloseTab(bundle, host string) (bool, error) {
	return closeIfHost(bundle, host, runScript)
}

// HideApp hides every running app with the bundle id. It needs no
// permission. false means no such app was running.
func HideApp(bundle string) (bool, error) {
	cs := C.CString(bundle)
	defer C.free(unsafe.Pointer(cs))
	return C.tock_hide_app(cs) > 0, nil
}
