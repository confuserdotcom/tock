//go:build windows

package collect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsSourceRunsAndStops(t *testing.T) {
	src, err := NewWindows(LinuxOptions{PollInterval: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- src.Run() }()
	time.Sleep(300 * time.Millisecond)
	src.Stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Stop")
	}
	n := 0
	for in := range src.Inputs() {
		if in.Sample != nil {
			n++
		}
	}
	if n == 0 {
		t.Fatal("no samples from the poll")
	}
}

func TestWindowsStopBeforeRun(t *testing.T) {
	src, _ := NewWindows(LinuxOptions{})
	src.Stop()
	done := make(chan error, 1)
	go func() { done <- src.Run() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run after Stop did not return")
	}
}

func TestWindowsIdle(t *testing.T) {
	b := &winBackend{}
	ms, ok := b.idleMS()
	if !ok || ms < 0 {
		t.Fatalf("idle = %d %v", ms, ok)
	}
}

func TestWindowsOwnProcessAndDescription(t *testing.T) {
	b := &winBackend{names: map[string]string{}, descs: map[string]string{}}
	p := sysProcs{b}
	exe, err := p.exe(uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	self, _ := os.Executable()
	if !strings.EqualFold(filepath.Clean(exe), filepath.Clean(self)) {
		t.Fatalf("exe = %q, want %q", exe, self)
	}
	notepad := filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe")
	if _, err := os.Stat(notepad); err != nil {
		t.Skip("no notepad.exe on this image")
	}
	if d := p.describe(notepad); d == "" {
		t.Fatal("notepad.exe has no description")
	}
	if _, err := p.exe(0x7ffffff0); err == nil {
		t.Fatal("a pid that doesn't exist opened")
	}
}
