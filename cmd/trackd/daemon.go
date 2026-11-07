package main

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"tock/internal/classify"
	"tock/internal/collect"
	"tock/internal/config"
	"tock/internal/span"
	"tock/internal/store"
	"tock/internal/update"
)

// Exit codes under launchd KeepAlive {SuccessfulExit=false} (D-17): 0 means
// "do not restart" (configuration problems a restart cannot fix), 1 means
// "restart me".
const (
	exitNoRestart = 0
	exitRestart   = 1
	// exitUpdated is runDaemon only: new binaries are in place and main
	// execs them; it is never a process exit code.
	exitUpdated = 3
)

const (
	defaultHeartbeatEvery = 10 * time.Second
	// dropWarnEveryMS rate-limits the dropped-sample warning.
	dropWarnEveryMS = 60_000
)

type daemonOpts struct {
	DataDir, StateDir string
	NewSource         func() (collect.Source, error)
	Stderr            io.Writer
	// HeartbeatEvery is the Merger.Tick period (default 10 s).
	HeartbeatEvery time.Duration
	// Now returns wall-clock unix ms (default time.Now().Round(0).UnixMilli).
	Now func() int64
	// ConfigDir holds rules.toml and config.toml. "" turns the focus engine
	// off (tests of the collector pipeline).
	ConfigDir string
	// FocusEvery is the focus engine period (default 5 s).
	FocusEvery time.Duration
	// Notifier sends focus notifications; nil drops them.
	Notifier Notifier
	// Remover blocks distractions for the focus guard; nil never blocks.
	Remover Remover
	// Tock is the tock binary session hooks run for Slack; "" = none.
	Tock string
	// Camera and Mic report a camera or a microphone in use; nil means
	// never. Either one means a call.
	Camera, Mic func() bool
	// Calendar reads calendar events; nil turns the calendar sync off.
	Calendar Calendar
	// CalendarPrompt allows the calendar permission prompt (trackd runs
	// under launchd).
	CalendarPrompt bool
	// CalendarEvery is the calendar loop period (default 1 min).
	CalendarEvery time.Duration
	// SyncEvery is the sync loop period (default 1 min).
	SyncEvery time.Duration
	// Exe is the running trackd, resolved. "" turns self-update off (main
	// sets it on Linux only); its folder is where updates are installed.
	Exe string
	// FreeEvery is how often freed memory goes back to the system (see
	// freeLoop); 0 never.
	FreeEvery time.Duration
	// SnapshotExe is the trackd binary run as the snapshot worker; ""
	// takes snapshots in this process (tests).
	SnapshotExe string
	// UpdateEvery is the update loop period (default 1 min).
	UpdateEvery time.Duration
	// OCR reads the front window's text; nil = screen reading is
	// unavailable (Linux, tests). SecureInput reports a password field
	// holding the keyboard; nil = never.
	OCR         collect.Capturer
	SecureInput func() bool
}

func wallNowMS() int64 { return time.Now().Round(0).UnixMilli() }

// runDaemon wires source, merger and store and returns the process exit code.
// It must be called on the main goroutine because src.Run does.
func runDaemon(o daemonOpts) int {
	if o.HeartbeatEvery <= 0 {
		o.HeartbeatEvery = defaultHeartbeatEvery
	}
	if o.Now == nil {
		o.Now = wallNowMS
	}
	syscall.Umask(0o077)

	logPath := filepath.Join(o.StateDir, "trackd.log")
	mkErr := os.MkdirAll(o.StateDir, 0o700)
	truncErr := truncateLogIfLarge(logPath, logMaxBytes)
	log := newLogger(o.Stderr)
	log.Info("trackd starting", "version", version, "pid", os.Getpid(),
		"db", filepath.Join(o.DataDir, "tock.db"))
	if mkErr != nil {
		log.Error("FATAL: cannot create state dir; not restarting", "dir", o.StateDir, "err", mkErr)
		return exitNoRestart
	}
	if truncErr != nil {
		log.Warn("log truncation failed", "path", logPath, "err", truncErr)
	}

	release, err := acquireLock(filepath.Join(o.StateDir, "trackd.lock"))
	if errors.Is(err, errLockHeld) {
		log.Error("FATAL: another trackd instance is already running; not restarting")
		return exitNoRestart
	}
	if err != nil {
		log.Error("FATAL: cannot take lock; not restarting", "err", err)
		return exitNoRestart
	}
	defer release()

	if err := os.MkdirAll(o.DataDir, 0o700); err != nil {
		log.Error("FATAL: cannot create data dir; not restarting", "dir", o.DataDir, "err", err)
		return exitNoRestart
	}
	st, err := store.Open(filepath.Join(o.DataDir, "tock.db"))
	if errors.Is(err, store.ErrSchemaTooNew) {
		log.Error("FATAL: database schema is newer than this trackd; not restarting", "err", err)
		return exitNoRestart
	}
	if err != nil {
		log.Error("FATAL: cannot open database; not restarting", "err", err)
		return exitNoRestart
	}
	rc := runWithStore(o, log, st)
	if err := st.Close(); err != nil {
		log.Error("close store", "err", err)
		rc = exitRestart
	}
	log.Info("trackd stopped", "exit", rc)
	return rc
}

// runWithStore recovers crashed spans and then runs the collector pipeline.
func runWithStore(o daemonOpts, log *slog.Logger, st *store.Store) int {
	n, err := st.Recover()
	if err != nil {
		log.Error("recover", "err", err)
		return exitRestart
	}
	log.Info("recovered open spans", "count", n)
	if err := st.SetState(store.KeyVersion, version, o.Now()); err != nil {
		log.Error("write version", "err", err)
		return exitRestart
	}
	next, err := st.NextID()
	if err != nil {
		log.Error("next id", "err", err)
		return exitRestart
	}
	afk := config.DefaultAFK()
	if o.ConfigDir != "" {
		c, err := config.Load(filepath.Join(o.ConfigDir, config.File))
		if err != nil {
			log.Warn("config", "err", err) // the defaults still apply
		}
		afk = c.AFK
	}
	m := span.New(mergerConfig(next, afk))

	src, err := o.NewSource()
	if err != nil {
		log.Error("FATAL: cannot create collector; not restarting", "err", err)
		return exitNoRestart
	}

	ticker := time.NewTicker(o.HeartbeatEvery)
	defer ticker.Stop()
	drops := &dropReporter{log: log}
	rec := &runRec{st: st, log: log, boot: bootMS, version: version, pid: os.Getpid()}
	p := pipe{
		rec:   rec,
		in:    src.Inputs(),
		m:     m,
		st:    st,
		tick:  ticker.C,
		now:   o.Now,
		watch: newStateWatcher(st, log, o.Now),
		log:   log,
		onTick: func(nowMS int64) {
			drops.report(src.Dropped(), nowMS)
		},
	}
	if o.ConfigDir != "" {
		p.hide = newHider(o.ConfigDir)
		p.ocr = newOCRCoord(&ocrCoord{
			cap: o.OCR, secure: o.SecureInput, st: st, log: log, now: o.Now,
			idleMS: span.DefaultConfig().IdleThreshold.Milliseconds(),
			load:   ocrLoader(o.ConfigDir),
		})
		defer p.ocr.close()
	}
	pipeErr := make(chan error, 1)
	go func() {
		err := p.run()
		if err != nil {
			src.Stop() // a dead pipeline must not leave the run loop spinning
		}
		pipeErr <- err
	}()
	var updated atomic.Bool
	stopUpdate := func() {} // called by hand once the source stops
	if o.ConfigDir != "" {
		f := newFocusLoop(o, log)
		sh := &shared{open: f.open}
		defer sh.close() // deferred first: closes after every loop stopped
		stopFocus := startFocus(f, sh, o.FocusEvery, log)
		defer stopFocus()
		every := o.SyncEvery
		if every <= 0 {
			every = syncEvery
		}
		stopSync := startLoop(sh.turn(&syncLoop{open: sh.opener, log: log}), every)
		defer stopSync()
		stopPrune := startLoop(sh.turn(&pruneLoop{open: sh.opener, log: log}), pruneEvery)
		defer stopPrune()
		snap := &snapshotLoop{open: sh.opener, log: log}
		if o.SnapshotExe != "" {
			snap.take = workerSnapshot(o.SnapshotExe, o.DataDir, o.ConfigDir)
		}
		stopSnap := startLoop(sh.turn(snap), snapshotEvery)
		defer stopSnap()
		if o.FreeEvery > 0 {
			stopFree := startLoop(freeLoop{}, o.FreeEvery)
			defer stopFree()
		}
		if o.Calendar != nil {
			every := o.CalendarEvery
			if every <= 0 {
				every = calendarCheckEvery
			}
			stopCal := startCalendar(&calendarLoop{cal: o.Calendar, st: st, cfgPath: filepath.Join(o.ConfigDir, config.File),
				canPrompt: o.CalendarPrompt, log: log, now: o.Now}, every)
			defer stopCal()
		}
		if o.Exe != "" {
			every := o.UpdateEvery
			if every <= 0 {
				every = syncEvery
			}
			stopUpdate = startLoop(&updateLoop{
				cfgPath: filepath.Join(o.ConfigDir, config.File),
				pinned:  filepath.Join(o.ConfigDir, update.PubFile),
				binDir:  filepath.Dir(o.Exe),
				arch:    runtime.GOARCH,
				run:     update.ParseRunning(version, builtAt),
				st:      st, log: log, now: o.Now,
				restart: func() { updated.Store(true); src.Stop() },
			}, every)
		}
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigs)
	go func() {
		if _, ok := <-sigs; ok {
			log.Info("signal received, stopping")
			src.Stop()
		}
	}()
	reason := store.RunSignal // a clean stop: a signal, or the source ended

	rc := exitNoRestart
	err = src.Run()
	// Stopping waits for a running tick, so the flag is settled after it.
	stopUpdate()
	if errors.Is(err, collect.ErrAXStale) {
		log.Warn("accessibility grant stale; exiting for restart")
		rc, reason = exitRestart, store.RunAXStale
	} else if err != nil {
		log.Error("source run", "err", err)
		rc, reason = exitRestart, store.RunError
	}
	if err := <-pipeErr; err != nil {
		log.Error("pipeline", "err", err)
		rc, reason = exitRestart, store.RunError
	}
	if updated.Load() {
		reason = store.RunUpdate
	}
	// The pipe has returned, so its row is ours to close.
	if err := rec.end(o.Now(), reason); err != nil {
		log.Error("end run row", "err", err)
	}
	if updated.Load() {
		return exitUpdated // the new binary restarts anyway
	}
	return rc
}

// mergerConfig is the daemon's span configuration: the Phase 1 defaults with
// titles compared after classify.Clean, so a spinner or counter change in a
// title does not split the span.
//
// While a video plays it waits [afk] video (read at start) before idle.
func mergerConfig(nextID int64, afk config.AFK) span.Config {
	cfg := span.DefaultConfig()
	cfg.NextID = nextID
	cfg.TitleKey = classify.Clean
	cfg.VideoIdleMax = time.Duration(afk.VideoMS) * time.Millisecond
	return cfg
}

// pipe feeds every input to the merger and applies the resulting ops. A tick
// applies a pending heartbeat so heartbeat_ts advances without new samples.
type pipe struct {
	in     <-chan span.Input
	m      *span.Merger
	st     *store.Store
	tick   <-chan time.Time // may be nil
	now    func() int64
	watch  *stateWatcher // may be nil
	onTick func(nowMS int64)
	ocr    *ocrCoord    // may be nil
	hide   *hider       // may be nil
	log    *slog.Logger // may be nil
	rec    *runRec      // may be nil
}

func (p pipe) run() error {
	if err := p.start(); err != nil {
		return err
	}
	for {
		select {
		case x, ok := <-p.in:
			if !ok {
				return p.flush()
			}
			if err := p.feed(x); err != nil {
				return err
			}
		case <-p.tick:
			if err := p.tickAt(p.now()); err != nil {
				return err
			}
		}
	}
}

// start reads config.toml once and records whether capture is paused,
// which also clears a pause an earlier run left behind, and opens this
// run's row in that state.
func (p pipe) start() error {
	if p.hide == nil && p.rec == nil {
		return nil
	}
	now := p.now()
	if p.hide != nil {
		p.hide.check(now)
		if err := p.recordPause(now); err != nil {
			return err
		}
	}
	return p.rec.begin(now, p.captureState())
}

// captureState is the run state: paused while config.toml does not load.
func (p pipe) captureState() string {
	if p.hide.paused() {
		return store.RunPaused
	}
	return store.RunRecording
}

// feed applies one input. While config.toml does not load, every input is
// dropped: nothing reaches the merger or the store.
func (p pipe) feed(x span.Input) error {
	if x.Sample != nil {
		if err := p.checkConfig(x.Sample.TS); err != nil {
			return err
		}
	}
	if p.hide.paused() {
		return nil
	}
	p.hide.apply(x.Sample)
	if x.Sample != nil && p.watch != nil {
		if err := p.watch.observe(x.Sample); err != nil {
			return err
		}
	}
	ops := p.m.Feed(x)
	if err := p.st.Apply(ops); err != nil {
		return err
	}
	if p.ocr != nil {
		p.ocr.observe(ops, x.Sample)
	}
	return nil
}

// tickAt moves this run's row on (see runRec.tick) and applies a pending
// heartbeat. While paused there is no open span, so the heartbeat is
// written as is: trackd is alive, only not recording.
func (p pipe) tickAt(now int64) error {
	if err := p.rec.tick(now); err != nil {
		return err
	}
	if err := p.checkConfig(now); err != nil {
		return err
	}
	if p.hide.paused() {
		if err := p.st.SetState(store.KeyHeartbeat, strconv.FormatInt(now, 10), now); err != nil {
			return err
		}
	} else if err := p.st.Apply(p.m.Tick(now)); err != nil {
		return err
	}
	if p.onTick != nil {
		p.onTick(now)
	}
	return nil
}

// flush closes the open span, as the input ends.
func (p pipe) flush() error { return p.st.Apply(p.m.Flush()) }

// checkConfig reads config.toml when due. Pausing closes the open span at
// its last sample, so nothing after the config broke is stored.
func (p pipe) checkConfig(ts int64) error {
	if !p.hide.check(ts) {
		return nil
	}
	if p.hide.paused() {
		if err := p.flush(); err != nil {
			return err
		}
	}
	if err := p.rec.setState(ts, p.captureState()); err != nil {
		return err
	}
	return p.recordPause(ts)
}

// recordPause writes the pause reason ("" = recording) for tock status,
// doctor, the TUI, the dashboard and the menu bar, and logs a change.
func (p pipe) recordPause(ts int64) error {
	why := p.hide.reason()
	if p.log != nil {
		if why != "" {
			p.log.Error("capture paused: config.toml does not load; nothing is recorded until it does", "err", why)
		} else {
			p.log.Info("capture running: config.toml loads")
		}
	}
	return p.st.SetState(store.KeyCapturePaused, why, ts)
}

// pipeline is pipe.run with wall-clock time and no state watching.
func pipeline(in <-chan span.Input, m *span.Merger, st *store.Store, tick <-chan time.Time) error {
	return pipe{in: in, m: m, st: st, tick: tick, now: wallNowMS}.run()
}

// stateWatcher writes permission state to daemon_state and logs it, once per
// change (D-19, D-20, TRK-07).
type stateWatcher struct {
	st      *store.Store
	log     *slog.Logger
	now     func() int64
	axKnown bool
	ax      bool
	auto    map[string]string
}

func newStateWatcher(st *store.Store, log *slog.Logger, now func() int64) *stateWatcher {
	return &stateWatcher{st: st, log: log, now: now, auto: make(map[string]string)}
}

func (w *stateWatcher) observe(s *span.Sample) error {
	if !w.axKnown || w.ax != s.AXTrusted {
		w.axKnown, w.ax = true, s.AXTrusted
		v := "0"
		if s.AXTrusted {
			v = "1"
		}
		if err := w.st.SetState(store.KeyAXTrusted, v, w.now()); err != nil {
			return err
		}
		if s.AXTrusted {
			w.log.Info("accessibility granted")
		} else {
			w.log.Warn("accessibility not granted; recording apps only",
				"settings", "System Settings > Privacy & Security > Accessibility > trackd")
		}
	}
	if s.Automation != "" && w.auto[s.BundleID] != s.Automation {
		w.auto[s.BundleID] = s.Automation
		if err := w.st.SetState(store.AutomationKey(s.BundleID), s.Automation, w.now()); err != nil {
			return err
		}
		if s.Automation == span.AutomationDenied {
			w.log.Warn("automation status changed", "browser", s.BundleID, "status", s.Automation,
				"settings", "System Settings > Privacy & Security > Automation > trackd")
		} else {
			w.log.Info("automation status changed", "browser", s.BundleID, "status", s.Automation)
		}
	}
	return nil
}

// dropReporter logs the dropped-sample total at most once per minute.
type dropReporter struct {
	log      *slog.Logger
	reported uint64
	lastMS   int64
	warned   bool
}

func (d *dropReporter) report(total uint64, nowMS int64) {
	if total <= d.reported || (d.warned && nowMS-d.lastMS < dropWarnEveryMS) {
		return
	}
	d.reported, d.lastMS, d.warned = total, nowMS, true
	d.log.Warn("samples dropped", "total", total)
}
