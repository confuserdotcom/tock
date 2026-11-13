package collect

import (
	"sync"
	"testing"
	"time"

	"tock/internal/span"
)

// hookBackend is a windowBackend that also names apps, knows the lock
// state and reports session events, as the Windows one does.
type hookBackend struct {
	mu     sync.Mutex
	class  string
	title  string
	locked bool
	emit   func(span.EventKind)
	quit   chan struct{}
	ready  chan struct{}
}

func newHookBackend() *hookBackend {
	return &hookBackend{class: "notepad", title: "a.txt", quit: make(chan struct{}), ready: make(chan struct{})}
}

func (b *hookBackend) active() (string, string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.class, b.title, nil
}
func (b *hookBackend) watch(func(string, string)) error {
	close(b.ready)
	<-b.quit
	return nil
}
func (b *hookBackend) idleMS() (int64, bool) { return 1500, true }
func (b *hookBackend) close()                { close(b.quit) }
func (b *hookBackend) appName(class string) string {
	if class == "notepad" {
		return "Notepad"
	}
	return ""
}
func (b *hookBackend) isLocked() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.locked
}
func (b *hookBackend) onEvent(f func(span.EventKind)) { b.emit = f }

func TestWinSourceUsesBackendHooks(t *testing.T) {
	b := newHookBackend()
	b.locked = true
	src := newWinSource(b, LinuxOptions{PollInterval: time.Hour, Proc: t.TempDir(), Now: func() int64 { return 7000 }})
	done := make(chan error, 1)
	go func() { done <- src.Run() }()
	<-b.ready
	b.emit(span.EventSleep)
	src.Stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var samples []*span.Sample
	var events []span.EventKind
	for in := range src.Inputs() {
		if in.Sample != nil {
			samples = append(samples, in.Sample)
		}
		if in.Event != nil {
			if in.Event.TS != 7000 {
				t.Errorf("event at %d", in.Event.TS)
			}
			events = append(events, in.Event.Kind)
		}
	}
	if len(samples) == 0 {
		t.Fatal("no sample")
	}
	s := samples[0]
	if s.BundleID != "notepad" || s.App != "Notepad" || !s.Locked || s.IdleS != 1.5 {
		t.Fatalf("sample = %+v", s)
	}
	if len(events) != 1 || events[0] != span.EventSleep {
		t.Fatalf("events = %v", events)
	}
}
