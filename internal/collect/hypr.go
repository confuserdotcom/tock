package collect

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tock/internal/span"
)

// Hyprland's bundle id and app name when no window has focus.
const (
	DesktopBundleID = "hyprland.desktop"
	DesktopApp      = "Desktop"
)

// IdleFile is the name of the file, in trackd's state dir, that the
// hypridle hooks (trackd idle / trackd active) write.
const IdleFile = "idle"

// lockers are the screen lockers whose process means the session is locked.
var lockers = []string{"hyprlock", "swaylock"}

// HyprOptions configures the Hyprland source. Empty fields take the
// defaults from the environment.
type HyprOptions struct {
	PollInterval time.Duration
	// Dir is the Hyprland instance dir holding .socket.sock and
	// .socket2.sock; "" finds it under $XDG_RUNTIME_DIR/hypr.
	Dir string
	// IdleFile is the idle file's path; "" means never idle.
	IdleFile string
	// Proc is the /proc dir scanned for screen lockers; "" means /proc.
	Proc string
	// Now returns wall-clock unix ms; nil means the system clock.
	Now func() int64
}

type hyprSource struct {
	opts    HyprOptions
	ch      chan span.Input
	dropped atomic.Uint64
	done    chan struct{}
	stop    sync.Once
	mu      sync.Mutex
	ev      net.Conn // the event socket, closed by Stop
}

// NewHyprland returns the Hyprland source. It fails when no Hyprland
// instance is found.
func NewHyprland(opts HyprOptions) (Source, error) {
	if opts.PollInterval <= 0 {
		opts.PollInterval = DefaultPollInterval
	}
	if opts.Proc == "" {
		opts.Proc = "/proc"
	}
	if opts.Now == nil {
		opts.Now = func() int64 { return time.Now().Round(0).UnixMilli() }
	}
	if opts.Dir == "" {
		d, err := HyprDir(os.Getenv("XDG_RUNTIME_DIR"), os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"))
		if err != nil {
			return nil, err
		}
		opts.Dir = d
	}
	return &hyprSource{opts: opts, ch: make(chan span.Input, ChanBuffer), done: make(chan struct{})}, nil
}

// HyprDir finds the Hyprland instance dir: the one named by sig, else the
// most recently started one that has an event socket.
func HyprDir(runtimeDir, sig string) (string, error) {
	if runtimeDir == "" {
		return "", errors.New("hyprland: XDG_RUNTIME_DIR is not set")
	}
	base := filepath.Join(runtimeDir, "hypr")
	if sig != "" {
		return filepath.Join(base, sig), nil
	}
	ents, err := os.ReadDir(base)
	if err != nil {
		return "", fmt.Errorf("hyprland: no instance: %w", err)
	}
	best, bestT := "", time.Time{}
	for _, e := range ents {
		d := filepath.Join(base, e.Name())
		fi, err := os.Stat(filepath.Join(d, ".socket2.sock"))
		if err != nil {
			continue
		}
		if best == "" || fi.ModTime().After(bestT) {
			best, bestT = d, fi.ModTime()
		}
	}
	if best == "" {
		return "", errors.New("hyprland: no running instance under " + base)
	}
	return best, nil
}

func (s *hyprSource) Inputs() <-chan span.Input { return s.ch }
func (s *hyprSource) Dropped() uint64           { return s.dropped.Load() }

func (s *hyprSource) Stop() {
	s.stop.Do(func() {
		close(s.done)
		s.mu.Lock()
		if s.ev != nil {
			s.ev.Close()
		}
		s.mu.Unlock()
	})
}

func (s *hyprSource) trySend(in span.Input) {
	select {
	case s.ch <- in:
	default:
		s.dropped.Add(1)
	}
}

// Run reads window events and polls until Stop, or until the event socket
// fails.
func (s *hyprSource) Run() error {
	defer close(s.ch)
	conn, err := net.Dial("unix", filepath.Join(s.opts.Dir, ".socket2.sock"))
	if err != nil {
		return fmt.Errorf("hyprland: event socket: %w", err)
	}
	s.mu.Lock()
	s.ev = conn
	s.mu.Unlock()
	select {
	case <-s.done: // stopped before the socket was stored
		conn.Close()
		return nil
	default:
	}

	evErr := make(chan error, 1)
	go func() { evErr <- s.events(conn) }()
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

// events turns activewindow events into activation samples.
func (s *hyprSource) events(r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		class, title, ok := parseActiveWindowEvent(sc.Text())
		if !ok {
			continue
		}
		smp := s.base()
		smp.Activation = true
		setWindow(smp, class, title)
		s.trySend(span.Input{Sample: smp})
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("hyprland: events: %w", err)
	}
	return errors.New("hyprland: event socket closed")
}

// poll asks Hyprland for the active window. A failed request sends
// nothing: the next poll or event carries on.
func (s *hyprSource) poll() {
	smp := s.base()
	class, title, err := s.activeWindow()
	if err != nil {
		return
	}
	setWindow(smp, class, title)
	s.trySend(span.Input{Sample: smp})
}

// base is a sample with the time, idle and lock state filled.
func (s *hyprSource) base() *span.Sample {
	now := s.opts.Now()
	return &span.Sample{
		TS:        now,
		IdleS:     idleSeconds(s.opts.IdleFile, now),
		Locked:    lockerRunning(s.opts.Proc),
		AXTrusted: true,
	}
}

func setWindow(smp *span.Sample, class, title string) {
	class = strings.ToValidUTF8(class, "�")
	if class == "" {
		smp.BundleID, smp.App = DesktopBundleID, DesktopApp
		return
	}
	smp.BundleID, smp.App = class, class
	t := strings.ToValidUTF8(title, "�")
	smp.Title = &t
}

// parseActiveWindowEvent reads "activewindow>>class,title". The class has
// no comma; the title may.
func parseActiveWindowEvent(line string) (class, title string, ok bool) {
	data, ok := strings.CutPrefix(line, "activewindow>>")
	if !ok {
		return "", "", false
	}
	class, title, _ = strings.Cut(data, ",")
	return class, title, true
}

// activeWindow sends j/activewindow on the request socket.
func (s *hyprSource) activeWindow() (class, title string, err error) {
	c, err := net.DialTimeout("unix", filepath.Join(s.opts.Dir, ".socket.sock"), time.Second)
	if err != nil {
		return "", "", err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	if _, err := c.Write([]byte("j/activewindow")); err != nil {
		return "", "", err
	}
	b, err := io.ReadAll(io.LimitReader(c, 1<<20))
	if err != nil {
		return "", "", err
	}
	return parseActiveWindow(b)
}

// parseActiveWindow reads the JSON of j/activewindow; {} means no window.
func parseActiveWindow(b []byte) (class, title string, err error) {
	var w struct {
		Class string `json:"class"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return "", "", fmt.Errorf("hyprland: activewindow: %w", err)
	}
	return w.Class, w.Title, nil
}

// idleSeconds reads the idle file: the unix ms idle began.
func idleSeconds(path string, nowMS int64) float64 {
	if path == "" {
		return 0
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	since, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil || since > nowMS {
		return 0
	}
	return float64(nowMS-since) / 1000
}

// lockerRunning reports a screen locker process under proc.
func lockerRunning(proc string) bool {
	ents, err := os.ReadDir(proc)
	if err != nil {
		return false
	}
	for _, e := range ents {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(proc, e.Name(), "comm"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(b))
		for _, l := range lockers {
			if name == l {
				return true
			}
		}
	}
	return false
}

// WriteIdle records that the user has been idle for after: the hypridle
// on-timeout hook. The file is replaced atomically.
func WriteIdle(path string, nowMS int64, after time.Duration) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.FormatInt(nowMS-after.Milliseconds(), 10)+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ClearIdle records that the user is back: the on-resume hook.
func ClearIdle(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
