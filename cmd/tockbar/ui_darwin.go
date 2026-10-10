//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework UserNotifications
#include <stdlib.h>
#include "ui_darwin.h"
*/
import "C"

import (
	"runtime"
	"unsafe"
)

// Pin main.main to the first OS thread: Cocoa runs there.
func init() { runtime.LockOSThread() }

// events carries Cocoa callbacks to the runner. The callbacks never block
// the main thread: a full channel drops the event.
var events = make(chan event, 64)

func send(ev event) {
	select {
	case events <- ev:
	default:
	}
}

//export goReady
func goReady() { send(event{kind: evReady}) }

//export goMenu
func goMenu(id *C.char) { send(event{kind: evMenu, id: C.GoString(id)}) }

//export goResponse
func goResponse(notice C.longlong, action *C.char) {
	send(event{kind: evResponse, notice: int64(notice), id: C.GoString(action)})
}

//export goAuth
func goAuth(status C.int) {
	access := "unknown"
	switch status {
	case 1:
		access = "granted"
	case 2:
		access = "denied"
	}
	send(event{kind: evAuth, id: access})
}

//export goPostFailed
func goPostFailed(notice C.longlong, msg *C.char) {
	send(event{kind: evPostFailed, notice: int64(notice), id: C.GoString(msg)})
}

type cocoa struct{}

func cstr(s string, f func(*C.char)) {
	c := C.CString(s)
	defer C.free(unsafe.Pointer(c))
	f(c)
}

func (cocoa) SetTitle(t string) { cstr(t, func(c *C.char) { C.bar_set_title(c) }) }
func (cocoa) SetMenu(s string)  { cstr(s, func(c *C.char) { C.bar_set_menu(c) }) }
func (cocoa) OpenURL(u string)  { cstr(u, func(c *C.char) { C.bar_open_url(c) }) }
func (cocoa) Quit()             { C.bar_quit() }

func (cocoa) CheckAuth(prompt bool) {
	p := C.int(0)
	if prompt {
		p = 1
	}
	C.bar_check_auth(p)
}

func (cocoa) Post(id int64, title, body, category, categories string) {
	cstr(title, func(t *C.char) {
		cstr(body, func(b *C.char) {
			cstr(category, func(c *C.char) {
				cstr(categories, func(cs *C.char) { C.bar_post(C.longlong(id), t, b, c, cs) })
			})
		})
	})
}

// runUI runs the runner on its own goroutine and Cocoa on this one, until
// the app quits.
func runUI(r *runner) {
	r.ui = cocoa{}
	go r.loop(events)
	C.bar_run()
}
