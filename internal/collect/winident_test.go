package collect

import (
	"testing"

	"tock/internal/span"
)

// fakeProcs is a process table: pid -> exe path, path -> description.
type fakeProcs struct {
	exes   map[uint32]string
	desc   map[string]string
	denied map[uint32]bool
}

func (f fakeProcs) exe(pid uint32) (string, error) {
	if f.denied[pid] {
		return "", errDenied
	}
	p, ok := f.exes[pid]
	if !ok {
		return "", errNoProcess
	}
	return p, nil
}

func (f fakeProcs) describe(path string) string { return f.desc[path] }

func TestIdentify(t *testing.T) {
	p := fakeProcs{
		exes: map[uint32]string{
			10: `C:\Program Files\Google\Chrome\Application\chrome.exe`,
			11: `C:\Windows\System32\notepad.exe`,
			12: `C:\Windows\System32\ApplicationFrameHost.exe`,
			13: `C:\Program Files\WindowsApps\Microsoft.WindowsCalculator\CalculatorApp.exe`,
			14: `C:\tools\ODD.EXE`,
		},
		desc: map[string]string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`:                      "Google Chrome",
			`C:\Windows\System32\notepad.exe`:                                            "Notepad",
			`C:\Program Files\WindowsApps\Microsoft.WindowsCalculator\CalculatorApp.exe`: "Calculator",
		},
		denied: map[uint32]bool{20: true},
	}
	cases := []struct {
		name       string
		pid        uint32
		children   []uint32
		class, app string
	}{
		{"description wins", 10, nil, "chrome", "Google Chrome"},
		{"plain", 11, nil, "notepad", "Notepad"},
		{"no version info: the exe name", 14, nil, "odd", "ODD"},
		{"store app behind its frame", 12, []uint32{12, 13}, "calculatorapp", "Calculator"},
		{"frame with no other child", 12, []uint32{12}, "applicationframehost", "ApplicationFrameHost"},
		{"elevated", 20, nil, "elevated", "Elevated app"},
		{"gone", 99, nil, "", ""},
	}
	for _, c := range cases {
		class, app := identify(c.pid, func() []uint32 { return c.children }, p)
		if class != c.class || app != c.app {
			t.Errorf("%s: identify = %q %q, want %q %q", c.name, class, app, c.class, c.app)
		}
	}
}

func TestIdentifyAsksForChildrenOnlyForTheFrame(t *testing.T) {
	p := fakeProcs{exes: map[uint32]string{1: `C:\a\b.exe`}}
	identify(1, func() []uint32 { t.Fatal("children asked for a normal app"); return nil }, p)
}

func TestSessionEvent(t *testing.T) {
	cases := []struct {
		msg    uint32
		wparam uintptr
		kind   span.EventKind
		ok     bool
	}{
		{wmWTSSessionChange, wtsSessionLock, span.EventLock, true},
		{wmWTSSessionChange, wtsSessionUnlock, span.EventUnlock, true},
		{wmWTSSessionChange, 1, 0, false}, // console connect
		{wmPowerBroadcast, pbtAPMSuspend, span.EventSleep, true},
		{wmPowerBroadcast, pbtAPMResumeAutomatic, span.EventWake, true},
		{wmPowerBroadcast, pbtAPMResumeSuspend, 0, false}, // follows the automatic one
		{wmPowerBroadcast, 0xA, 0, false},                 // power status change
		{0x0001, 0, 0, false},
	}
	for _, c := range cases {
		k, ok := sessionEvent(c.msg, c.wparam)
		if k != c.kind || ok != c.ok {
			t.Errorf("sessionEvent(%#x, %#x) = %v %v, want %v %v", c.msg, c.wparam, k, ok, c.kind, c.ok)
		}
	}
}

func TestDesktopWindowClasses(t *testing.T) {
	for _, c := range []string{"Progman", "WorkerW", "Shell_TrayWnd"} {
		if !desktopWindow(c) {
			t.Errorf("%s is the desktop", c)
		}
	}
	if desktopWindow("Chrome_WidgetWin_1") {
		t.Error("a browser is not the desktop")
	}
}

func TestWindowsBrowsersUseTheExtension(t *testing.T) {
	for _, c := range []string{"chrome", "msedge", "brave", "firefox", "vivaldi"} {
		if !browserClasses[c] {
			t.Errorf("%s: no front tab from the extension", c)
		}
	}
}
