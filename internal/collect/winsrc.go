package collect

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tock/internal/fronttab"
	"tock/internal/span"
)

// windowBackend is a Linux desktop seen as its focused window: Sway/i3
// over their IPC, X11 over its wire protocol.
type windowBackend interface {
	// active asks for the focused window; class "" means none (the desktop).
	active() (class, title string, err error)
	// watch blocks, calling focus on each change of focused window, until
	// the backend is closed or its connection fails.
	watch(focus func(class, title string)) error
	// idleMS is the time since the last input when the backend knows it
	// (X11's screensaver), else ok is false and the idle file says.
	idleMS() (ms int64, ok bool)
	close()
}

// Optional windowBackend abilities, used when a backend has them (the
// Windows one has all three).
type (
	// appNamer gives the display name for a class ("Google Chrome" for
	// chrome); "" keeps the class.
	appNamer interface{ appName(class string) string }
	// lockKnower says whether the session is locked, instead of looking
	// for a screen locker process.
	lockKnower interface{ isLocked() bool }
	// eventSource reports sleep, wake, lock and unlock to f, from the time
	// watch runs.
	eventSource interface{ onEvent(f func(span.EventKind)) }
)

// LinuxOptions configures the Sway/i3 and X11 sources. Empty fields take
// the defaults from the environment.
type LinuxOptions struct {
	PollInterval time.Duration
	// IdleFile is the idle file swayidle's hooks write; "" means never idle
	// (X11 asks the X server instead).
	IdleFile string
	// Proc is the /proc dir scanned for screen lockers; "" means /proc.
	Proc string
	// BrowserFile is the front-tab file the browser host writes.
	BrowserFile string
	// Now returns wall-clock unix ms; nil means the system clock.
	Now func() int64
}

func (o *LinuxOptions) defaults() {
	if o.PollInterval <= 0 {
		o.PollInterval = DefaultPollInterval
	}
	if o.Proc == "" {
		o.Proc = "/proc"
	}
	if o.Now == nil {
		o.Now = func() int64 { return time.Now().Round(0).UnixMilli() }
	}
}

// winSource turns a windowBackend into a Source: focus changes are
// activations, and a poll every PollInterval keeps titles and idle fresh.
type winSource struct {
	opts    LinuxOptions
	b       windowBackend
	ch      chan span.Input
	dropped atomic.Uint64
	done    chan struct{}
	stop    sync.Once
	since   atomic.Int64
}

func newWinSource(b windowBackend, opts LinuxOptions) *winSource {
	opts.defaults()
	return &winSource{opts: opts, b: b, ch: make(chan span.Input, ChanBuffer), done: make(chan struct{})}
}

func (s *winSource) Inputs() <-chan span.Input { return s.ch }
func (s *winSource) Dropped() uint64           { return s.dropped.Load() }

func (s *winSource) Stop() {
	s.stop.Do(func() {
		close(s.done)
		s.b.close()
	})
}

func (s *winSource) trySend(in span.Input) {
	select {
	case s.ch <- in:
	default:
		s.dropped.Add(1)
	}
}

// Run watches focus changes and polls until Stop, or until the backend's
// connection fails.
func (s *winSource) Run() error {
	defer close(s.ch)
	select {
	case <-s.done:
		return nil
	default:
	}
	if es, ok := s.b.(eventSource); ok {
		es.onEvent(func(k span.EventKind) {
			s.trySend(span.Input{Event: &span.Event{TS: s.opts.Now(), Kind: k}})
		})
	}
	evErr := make(chan error, 1)
	go func() {
		evErr <- s.b.watch(func(class, title string) {
			smp := s.base()
			smp.Activation = true
			s.since.Store(smp.TS)
			s.fill(smp, class, title)
			s.trySend(span.Input{Sample: smp})
		})
	}()
	t := time.NewTicker(s.opts.PollInterval)
	defer t.Stop()
	s.poll()
	for {
		select {
		case <-s.done:
			<-evErr
			return nil
		case err := <-evErr:
			select {
			case <-s.done:
				return nil
			default:
			}
			return err
		case <-t.C:
			s.poll()
		}
	}
}

func (s *winSource) poll() {
	class, title, err := s.b.active()
	if err != nil {
		return
	}
	smp := s.base()
	s.fill(smp, class, title)
	s.trySend(span.Input{Sample: smp})
}

func (s *winSource) fill(smp *span.Sample, class, title string) {
	setWindow(smp, class, title)
	if n, ok := s.b.(appNamer); ok && class != "" {
		if app := n.appName(class); app != "" {
			smp.App = app
		}
	}
	if s.opts.BrowserFile != "" && browserClasses[strings.ToLower(smp.BundleID)] {
		if st, ok := fronttab.Read(s.opts.BrowserFile); ok {
			applyFrontTab(smp, st, s.since.Load())
		}
	}
}

func (s *winSource) base() *span.Sample {
	now := s.opts.Now()
	idle := idleSeconds(s.opts.IdleFile, now)
	if ms, ok := s.b.idleMS(); ok {
		idle = float64(ms) / 1000
	}
	var locked bool
	if l, ok := s.b.(lockKnower); ok {
		locked = l.isLocked()
	} else {
		locked = lockerRunning(s.opts.Proc)
	}
	return &span.Sample{TS: now, IdleS: idle, Locked: locked, AXTrusted: true}
}
