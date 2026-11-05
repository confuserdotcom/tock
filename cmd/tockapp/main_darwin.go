//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
#include "app_darwin.h"
*/
import "C"

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

func init() { runtime.LockOSThread() } // AppKit runs on the main thread

func dataDir() string {
	if d := os.Getenv("TOCK_DATA_DIR"); d != "" {
		return d
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".local", "share", "tock")
}

// installedVersion is the version of the menu bar app tock setup put in
// place, "" when there is none.
func installedVersion() string {
	bar := filepath.Join(dataDir(), "tock.app", "Contents", "MacOS", "tockbar")
	out, err := exec.Command(bar, "version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func main() {
	self, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	_, perr := os.Stat(filepath.Join(payloadDir(self), "tock"))
	p := decide(self, version, installedVersion(), perr == nil)
	if p.kind == "setup" {
		// You let this app open with Open Anyway; the tools inside it came
		// down with the same quarantine and would each be stopped again.
		_ = exec.Command("/usr/bin/xattr", "-dr", "com.apple.quarantine", payloadDir(self)).Run()
	}

	cs := func(s string) *C.char { return C.CString(s) }
	argv := make([]*C.char, len(p.cmd)+1)
	for i, a := range p.cmd {
		argv[i] = cs(a)
	}
	kind, title, text, ok := cs(p.kind), cs(p.title), cs(p.text), cs(p.ok)
	code := C.launcherRun(kind, title, text, ok, (**C.char)(unsafe.Pointer(&argv[0])), C.int(len(p.cmd)))
	os.Exit(int(code))
}
