package collect

import (
	"errors"
	"strings"

	"tock/internal/span"
)

// The portable half of the Windows collector: how a foreground window's
// process becomes a class and an app name, and which window messages are
// session events. It builds everywhere so the logic is tested everywhere;
// collect_windows.go only makes the system calls.

var (
	// errDenied: the process can't be opened (a protected process).
	errDenied = errors.New("collect: process access denied")
	// errNoProcess: the process is gone.
	errNoProcess = errors.New("collect: no such process")
)

// procTable reads a process's executable and that file's description.
type procTable interface {
	exe(pid uint32) (string, error)
	describe(path string) string
}

// Class and app for a window whose process tock may not open.
const (
	ElevatedClass = "elevated"
	ElevatedApp   = "Elevated app"
)

// frameHost hosts Store (UWP) apps' windows; the app itself is a child.
const frameHost = "applicationframehost"

// identify returns the class (the exe's name, lowercased, without .exe)
// and the app name (the exe's description, else its name) of the process
// that owns the foreground window. children lists the pids owning that
// window's child windows; it is called only for a Store app's frame. A
// gone process gives "", "".
func identify(pid uint32, children func() []uint32, p procTable) (class, app string) {
	path, err := p.exe(pid)
	if errors.Is(err, errDenied) {
		return ElevatedClass, ElevatedApp
	}
	if err != nil {
		return "", ""
	}
	if strings.EqualFold(exeName(path), frameHost) {
		for _, c := range children() {
			if c == pid {
				continue
			}
			if cp, err := p.exe(c); err == nil {
				path = cp
				break
			}
		}
	}
	name := exeName(path)
	app = strings.TrimSpace(p.describe(path))
	if app == "" {
		app = name
	}
	return strings.ToLower(name), app
}

// exeName is a Windows path's last element without .exe, on any OS.
func exeName(path string) string {
	if i := strings.LastIndexAny(path, `\/`); i >= 0 {
		path = path[i+1:]
	}
	if len(path) > 4 && strings.EqualFold(path[len(path)-4:], ".exe") {
		path = path[:len(path)-4]
	}
	return path
}

// desktopWindow reports whether a window class is the desktop or the
// taskbar: focus there is "nothing in front", like Linux's empty class.
func desktopWindow(class string) bool {
	switch class {
	case "Progman", "WorkerW", "Shell_TrayWnd", "Shell_SecondaryTrayWnd":
		return true
	}
	return false
}

// Window messages and their parameters (winuser.h, wtsapi32.h).
const (
	wmQueryEndSession  = 0x0011
	wmEndSession       = 0x0016
	wmPowerBroadcast   = 0x0218
	wmWTSSessionChange = 0x02B1

	pbtAPMSuspend         = 0x4
	pbtAPMResumeSuspend   = 0x7
	pbtAPMResumeAutomatic = 0x12

	wtsSessionLock   = 0x7
	wtsSessionUnlock = 0x8
)

// sessionEvent maps a window message to a sleep, wake, lock or unlock.
// Wake is the automatic resume, which every resume sends; the "resume
// suspend" that follows a user's return would be a second wake.
func sessionEvent(msg uint32, wparam uintptr) (span.EventKind, bool) {
	switch msg {
	case wmWTSSessionChange:
		switch wparam {
		case wtsSessionLock:
			return span.EventLock, true
		case wtsSessionUnlock:
			return span.EventUnlock, true
		}
	case wmPowerBroadcast:
		switch wparam {
		case pbtAPMSuspend:
			return span.EventSleep, true
		case pbtAPMResumeAutomatic:
			return span.EventWake, true
		}
	}
	return 0, false
}
