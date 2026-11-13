//go:build windows

package stopsignal

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// userSID is the current user's SID, which names the event and is the
// only account its security descriptor lets in.
func userSID() (string, error) {
	tok, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	defer tok.Close()
	u, err := tok.GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String(), nil
}

func eventName(name, sid string) *uint16 {
	return windows.StringToUTF16Ptr(`Local\tock-` + name + `-stop-` + sid)
}

func listen(name string) (<-chan struct{}, func(), error) {
	sid, err := userSID()
	if err != nil {
		return nil, nil, fmt.Errorf("stopsignal: user: %w", err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + sid + ")")
	if err != nil {
		return nil, nil, fmt.Errorf("stopsignal: security descriptor: %w", err)
	}
	sa := windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(sa))
	ev, err := windows.CreateEvent(&sa, 1, 0, eventName(name, sid))
	if ev == 0 {
		return nil, nil, fmt.Errorf("stopsignal: create: %w", err)
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		_ = windows.ResetEvent(ev) // a stop for an earlier listener
	}
	cancel, err := windows.CreateEvent(nil, 1, 0, nil)
	if cancel == 0 {
		windows.CloseHandle(ev)
		return nil, nil, fmt.Errorf("stopsignal: create: %w", err)
	}
	ch := make(chan struct{})
	go func() {
		defer windows.CloseHandle(ev)
		defer windows.CloseHandle(cancel)
		r, _ := windows.WaitForMultipleObjects([]windows.Handle{ev, cancel}, false, windows.INFINITE)
		if r == windows.WAIT_OBJECT_0 {
			close(ch)
		}
	}()
	var once sync.Once
	return ch, func() { once.Do(func() { _ = windows.SetEvent(cancel) }) }, nil
}

func send(name string) error {
	sid, err := userSID()
	if err != nil {
		return fmt.Errorf("stopsignal: user: %w", err)
	}
	ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, eventName(name, sid))
	if err != nil {
		return ErrNotListening
	}
	defer windows.CloseHandle(ev)
	if err := windows.SetEvent(ev); err != nil {
		return fmt.Errorf("stopsignal: send: %w", err)
	}
	return nil
}
