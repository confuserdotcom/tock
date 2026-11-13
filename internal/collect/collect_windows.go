//go:build windows

package collect

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"

	"tock/internal/span"
)

// The Windows collector: a winSource over this backend. A hidden top-level
// window (never shown) receives lock/unlock, sleep/wake and logoff; a
// WinEvent hook reports focus changes; titles and idle come from the
// source's poll. No cgo: plain user32/wtsapi32/version calls.

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	wtsapi32 = windows.NewLazySystemDLL("wtsapi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procSetWinEventHook          = user32.NewProc("SetWinEventHook")
	procUnhookWinEvent           = user32.NewProc("UnhookWinEvent")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetClassNameW            = user32.NewProc("GetClassNameW")
	procEnumChildWindows         = user32.NewProc("EnumChildWindows")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")
	procPostThreadMessageW       = user32.NewProc("PostThreadMessageW")
	procPostQuitMessage          = user32.NewProc("PostQuitMessage")
	procRegisterClassExW         = user32.NewProc("RegisterClassExW")
	procCreateWindowExW          = user32.NewProc("CreateWindowExW")
	procDestroyWindow            = user32.NewProc("DestroyWindow")
	procDefWindowProcW           = user32.NewProc("DefWindowProcW")
	procGetLastInputInfo         = user32.NewProc("GetLastInputInfo")
	procWTSRegister              = wtsapi32.NewProc("WTSRegisterSessionNotification")
	procWTSUnRegister            = wtsapi32.NewProc("WTSUnRegisterSessionNotification")
	procGetTickCount64           = kernel32.NewProc("GetTickCount64")
)

const (
	eventSystemForeground = 0x0003
	wineventOutOfContext  = 0x0000
	wineventSkipOwnProc   = 0x0002
	wmQuit                = 0x0012
	wsExToolWindow        = 0x00000080
	notifyForThisSession  = 0
	maxTitle              = 1024
	maxDescriptions       = 512
)

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
	private uint32
}

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   windows.Handle
	icon       windows.Handle
	cursor     windows.Handle
	background windows.Handle
	menuName   *uint16
	className  *uint16
	iconSm     windows.Handle
}

type lastInputInfo struct {
	size uint32
	time uint32
}

// sessionEnding is set when Windows asks to end the session (logoff,
// restart, shut down): trackd's stop after it is not a crash.
var sessionEnding atomic.Bool

// WindowsSessionEnding reports whether Windows has said the session ends.
func WindowsSessionEnding() bool { return sessionEnding.Load() }

// NewWindows returns the Windows source. opts.BrowserFile is the front-tab
// file the extension's native host writes; IdleFile and Proc are unused.
func NewWindows(opts LinuxOptions) (Source, error) {
	b := &winBackend{names: map[string]string{}, descs: map[string]string{}}
	return newWinSource(b, opts), nil
}

// current is the backend whose message thread is running: window and
// hook callbacks are process-wide, and trackd runs one source.
var current atomic.Pointer[winBackend]

var (
	callbacksOnce sync.Once
	wndProcCB     uintptr
	winEventCB    uintptr
	enumChildCB   uintptr
	className     = windows.StringToUTF16Ptr("tockCollect")
	classErr      error
)

func setupCallbacks() {
	callbacksOnce.Do(func() {
		wndProcCB = windows.NewCallback(wndProc)
		winEventCB = windows.NewCallback(winEventProc)
		enumChildCB = windows.NewCallback(enumChildProc)
		wc := wndClassEx{wndProc: wndProcCB, className: className}
		wc.size = uint32(unsafe.Sizeof(wc))
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			classErr = fmt.Errorf("collect: register window class: %w", err)
		}
	})
}

type winBackend struct {
	tid    atomic.Uint32
	closed atomic.Bool
	locked atomic.Bool
	focus  func(class, title string)
	emit   func(span.EventKind)

	mu    sync.Mutex
	names map[string]string // class -> app name, from the last identify
	descs map[string]string // exe path -> description
}

func (b *winBackend) onEvent(f func(span.EventKind)) { b.emit = f }
func (b *winBackend) isLocked() bool                 { return b.locked.Load() }

func (b *winBackend) appName(class string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.names[class]
}

func (b *winBackend) idleMS() (int64, bool) {
	li := lastInputInfo{size: uint32(unsafe.Sizeof(lastInputInfo{}))}
	if r, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&li))); r == 0 {
		return 0, false
	}
	tick, _, _ := procGetTickCount64.Call()
	// dwTime is GetTickCount's 32 bits: subtract in 32 bits, wrap included.
	return int64(uint32(tick) - li.time), true
}

func (b *winBackend) active() (string, string, error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "", "", nil
	}
	if desktopWindow(windowClass(hwnd)) {
		return "", "", nil
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	class, app := identify(pid, func() []uint32 { return childPIDs(hwnd) }, sysProcs{b})
	if class == "" {
		return "", "", nil
	}
	b.mu.Lock()
	if len(b.names) > maxDescriptions {
		clear(b.names)
	}
	b.names[class] = app
	b.mu.Unlock()
	return class, windowTitle(hwnd), nil
}

func (b *winBackend) watch(focus func(class, title string)) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	setupCallbacks()
	if classErr != nil {
		return classErr
	}
	b.focus = focus
	if !current.CompareAndSwap(nil, b) {
		return errors.New("collect: a Windows source is already running")
	}
	defer current.Store(nil)

	// A top-level window, never shown: only those get power and session
	// broadcasts. Creating it also gives this thread its message queue.
	title := windows.StringToUTF16Ptr("tock")
	hwnd, _, err := procCreateWindowExW.Call(wsExToolWindow, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		0, 0, 0, 0, 0, 0, 0, 0, 0)
	if hwnd == 0 {
		return fmt.Errorf("collect: create window: %w", err)
	}
	defer procDestroyWindow.Call(hwnd)
	b.tid.Store(windows.GetCurrentThreadId())
	if b.closed.Load() {
		return nil
	}
	if r, _, err := procWTSRegister.Call(hwnd, notifyForThisSession); r == 0 {
		return fmt.Errorf("collect: session notifications: %w", err)
	}
	defer procWTSUnRegister.Call(hwnd)
	hook, _, err := procSetWinEventHook.Call(eventSystemForeground, eventSystemForeground, 0, winEventCB, 0, 0,
		wineventOutOfContext|wineventSkipOwnProc)
	if hook == 0 {
		return fmt.Errorf("collect: focus hook: %w", err)
	}
	defer procUnhookWinEvent.Call(hook)

	var m msg
	for {
		r, _, err := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		switch int32(r) {
		case 0: // WM_QUIT
			return nil
		case -1:
			return fmt.Errorf("collect: message loop: %w", err)
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (b *winBackend) close() {
	b.closed.Store(true)
	if tid := b.tid.Load(); tid != 0 {
		procPostThreadMessageW.Call(uintptr(tid), wmQuit, 0, 0)
	}
}

func wndProc(hwnd, message, wparam, lparam uintptr) uintptr {
	b := current.Load()
	m := uint32(message)
	if k, ok := sessionEvent(m, wparam); ok && b != nil {
		switch k {
		case span.EventLock:
			b.locked.Store(true)
		case span.EventUnlock:
			b.locked.Store(false)
		}
		if b.emit != nil {
			b.emit(k)
		}
	}
	switch m {
	case wmPowerBroadcast:
		return 1
	case wmQueryEndSession:
		sessionEnding.Store(true)
		return 1 // never hold up a logoff or shut down
	case wmEndSession:
		if wparam != 0 { // it does end: stop now, the store is closed cleanly
			sessionEnding.Store(true)
			procPostQuitMessage.Call(0)
		} else {
			sessionEnding.Store(false) // cancelled
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, message, wparam, lparam)
	return r
}

func winEventProc(hook, event, hwnd, idObject, idChild, thread, ms uintptr) uintptr {
	b := current.Load()
	if b == nil || b.focus == nil || uint32(event) != eventSystemForeground {
		return 0
	}
	class, title, err := b.active()
	if err == nil {
		b.focus(class, title)
	}
	return 0
}

// childPIDs lists the processes owning hwnd's child windows (a Store
// app's frame holds the app's window). Calls run on one thread at a time.
var (
	childMu   sync.Mutex
	childList []uint32
)

func childPIDs(hwnd uintptr) []uint32 {
	childMu.Lock()
	defer childMu.Unlock()
	childList = childList[:0]
	procEnumChildWindows.Call(hwnd, enumChildCB, 0)
	return append([]uint32(nil), childList...)
}

func enumChildProc(hwnd, _ uintptr) uintptr {
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != 0 && len(childList) < 64 {
		childList = append(childList, pid)
	}
	return 1
}

func windowClass(hwnd uintptr) string {
	var buf [256]uint16
	n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

func windowTitle(hwnd uintptr) string {
	n, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if n == 0 {
		return ""
	}
	if n > maxTitle {
		n = maxTitle
	}
	buf := make([]uint16, n+1)
	got, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:got])
}

// sysProcs is the procTable of the running system.
type sysProcs struct{ b *winBackend }

func (sysProcs) exe(pid uint32) (string, error) {
	if pid == 0 {
		return "", errNoProcess
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return "", errDenied
	}
	if err != nil {
		return "", errNoProcess
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return "", errDenied
		}
		return "", errNoProcess
	}
	return windows.UTF16ToString(buf[:n]), nil
}

func (p sysProcs) describe(path string) string {
	p.b.mu.Lock()
	d, ok := p.b.descs[path]
	p.b.mu.Unlock()
	if ok {
		return d
	}
	d = fileDescription(path)
	p.b.mu.Lock()
	if len(p.b.descs) > maxDescriptions {
		clear(p.b.descs)
	}
	p.b.descs[path] = d
	p.b.mu.Unlock()
	return d
}

// fileDescription is the FileDescription of an exe's version resource, ""
// when it has none.
func fileDescription(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return ""
	}
	info := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&info[0])); err != nil {
		return ""
	}
	var langs []string
	var tp unsafe.Pointer
	var tn uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&info[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&tp), &tn); err == nil && tn >= 4 {
		pairs := unsafe.Slice((*uint16)(tp), tn/2)
		for i := 0; i+1 < len(pairs); i += 2 {
			langs = append(langs, fmt.Sprintf("%04x%04x", pairs[i], pairs[i+1]))
		}
	}
	langs = append(langs, "040904b0", "040904e4")
	for _, l := range langs {
		var vp unsafe.Pointer
		var vn uint32
		if err := windows.VerQueryValue(unsafe.Pointer(&info[0]), `\StringFileInfo\`+l+`\FileDescription`, unsafe.Pointer(&vp), &vn); err == nil && vn > 0 {
			return windows.UTF16ToString(unsafe.Slice((*uint16)(vp), vn))
		}
	}
	return ""
}
