package flock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func open(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestExclusiveRefusesSecond(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	a, b := open(t, p), open(t, p)
	if err := Lock(a, false, false); err != nil {
		t.Fatalf("first exclusive: %v", err)
	}
	if err := Lock(b, false, false); !errors.Is(err, ErrHeld) {
		t.Fatalf("second exclusive = %v, want ErrHeld", err)
	}
	if err := Lock(b, true, false); !errors.Is(err, ErrHeld) {
		t.Fatalf("shared under exclusive = %v, want ErrHeld", err)
	}
}

func TestSharedTogether(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	a, b, c := open(t, p), open(t, p), open(t, p)
	if err := Lock(a, true, false); err != nil {
		t.Fatal(err)
	}
	if err := Lock(b, true, false); err != nil {
		t.Fatalf("second shared: %v", err)
	}
	if err := Lock(c, false, false); !errors.Is(err, ErrHeld) {
		t.Fatalf("exclusive under shared = %v, want ErrHeld", err)
	}
}

func TestCloseReleases(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	a := open(t, p)
	if err := Lock(a, false, false); err != nil {
		t.Fatal(err)
	}
	_ = a.Close()
	if err := Lock(open(t, p), false, false); err != nil {
		t.Fatalf("after close: %v", err)
	}
}

func TestWaitBlocksUntilReleased(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	a, b := open(t, p), open(t, p)
	if err := Lock(a, false, false); err != nil {
		t.Fatal(err)
	}
	got := make(chan error, 1)
	go func() { got <- Lock(b, true, true) }()
	select {
	case err := <-got:
		t.Fatalf("waiting lock returned early: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	_ = a.Close()
	select {
	case err := <-got:
		if err != nil {
			t.Fatalf("waiting lock: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiting lock never returned")
	}
}

func TestAliveSelfAndGone(t *testing.T) {
	if !Alive(os.Getpid()) {
		t.Fatal("this process is not alive")
	}
	if Alive(0x7ffffff0) {
		t.Fatal("an unused pid is alive")
	}
}
