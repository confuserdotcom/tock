//go:build darwin && cgo

package main

/*
#include <stdlib.h>
#include "save_darwin.h"
*/
import "C"

import (
	"syscall"
	"unsafe"
)

// Go faces of save_darwin.c, for its tests.

const (
	navAllow    = int(C.TOCK_NAV_ALLOW)
	navBrowser  = int(C.TOCK_NAV_BROWSER)
	navDownload = int(C.TOCK_NAV_DOWNLOAD)
)

func navPolicy(host string, hasHost, download bool) int {
	var h *C.char
	if hasHost {
		h = C.CString(host)
		defer C.free(unsafe.Pointer(h))
	}
	d := C.int(0)
	if download {
		d = 1
	}
	return int(C.tock_nav_policy(h, d))
}

func saveName(suggested string) string {
	s := C.CString(suggested)
	defer C.free(unsafe.Pointer(s))
	var out [1024]C.char
	C.tock_save_name(s, &out[0], C.size_t(len(out)))
	return C.GoString(&out[0])
}

func finishSave(tmp, dest string) error {
	t, d := C.CString(tmp), C.CString(dest)
	defer C.free(unsafe.Pointer(t))
	defer C.free(unsafe.Pointer(d))
	if e := C.tock_finish_save(t, d); e != 0 {
		return syscall.Errno(e)
	}
	return nil
}
