package stopsignal

import (
	"errors"
	"runtime"
	"testing"
	"time"
)

func TestSendReachesListener(t *testing.T) {
	if runtime.GOOS != "windows" {
		if _, _, err := Listen("t"); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("Listen off Windows = %v", err)
		}
		if err := Send("t"); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("Send off Windows = %v", err)
		}
		return
	}
	name := "test-" + time.Now().Format("150405.000000")
	if err := Send(name); !errors.Is(err, ErrNotListening) {
		t.Fatalf("Send with nobody listening = %v", err)
	}
	ch, release, err := Listen(name)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	select {
	case <-ch:
		t.Fatal("stopped before Send")
	case <-time.After(50 * time.Millisecond):
	}
	if err := Send(name); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("Send never arrived")
	}
}

func TestReleaseIsNotAStop(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	ch, release, err := Listen("test-release-" + time.Now().Format("150405.000000"))
	if err != nil {
		t.Fatal(err)
	}
	release()
	release() // twice is fine
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("a value after release")
		}
		t.Fatal("release closed the channel like a stop")
	case <-time.After(100 * time.Millisecond):
	}
}
