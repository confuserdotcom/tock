// Command trackd is the tock tracking daemon.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"tock/internal/collect"
	"tock/internal/fronttab"
	"tock/internal/paths"
	"tock/internal/store"
)

// version is set via -ldflags "-X main.version=...".
var version = "dev"

// builtAt is set via -ldflags "-X main.builtAt=..." (RFC 3339, UTC); "" when unknown.
var builtAt = ""

func home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return os.Getenv("HOME")
}

func dataDir() string   { return paths.Default().Data }
func configDir() string { return paths.Default().Config }
func stateDir() string  { return paths.Default().State }

// newSource builds the collector: on Linux the session's desktop
// (Hyprland, Sway/i3 or X11), else macOS.
// Permission prompts are allowed only when launchd (or open) started
// trackd: a terminal-launched process would attribute them to the terminal.
func newSource() (collect.Source, error) {
	if runtime.GOOS == "windows" {
		fmt.Fprintln(os.Stderr, "trackd: reading the Windows session")
		return collect.NewWindows(collect.LinuxOptions{BrowserFile: filepath.Join(stateDir(), fronttab.File)})
	}
	if runtime.GOOS == "linux" {
		idle, browser := filepath.Join(stateDir(), collect.IdleFile), filepath.Join(stateDir(), fronttab.File)
		src, desktop, err := collect.NewLinux(collect.HyprOptions{IdleFile: idle, BrowserFile: browser},
			collect.LinuxOptions{IdleFile: idle, BrowserFile: browser})
		if err == nil {
			fmt.Fprintf(os.Stderr, "trackd: reading the %s session\n", desktop)
		}
		return src, err
	}
	underLaunchd := os.Getppid() == 1
	return collect.NewDarwin(collect.Options{
		PollInterval:  collect.DefaultPollInterval,
		PromptAX:      underLaunchd,
		AskAutomation: underLaunchd,
	})
}

// memoryLimit is trackd's soft Go memory limit (runtime/debug).
const memoryLimit = 32 << 20

// gcPercent is trackd's GOGC; cacheKiB its SQLite page cache per
// connection. Measured on a month of history: footprint 36 MB at the
// defaults, 28 MB with these, the same CPU.
const (
	gcPercent = 50
	cacheKiB  = 512
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "ocr-worker" {
		os.Exit(runOCRWorker(os.Args[2:], os.Stdout))
	}
	if len(os.Args) > 1 && os.Args[1] == "migrate-worker" {
		os.Exit(runMigrateWorker(os.Args[2:], os.Stdout))
	}
	if len(os.Args) > 1 && os.Args[1] == "snapshot-worker" {
		os.Exit(runSnapshotWorker(os.Args[2:], os.Stdout))
	}
	if host, _ := browserHostCaller(os.Args); host {
		os.Exit(runBrowserHost(os.Args, stateDir()))
	}
	if len(os.Args) > 1 {
		var run func() error
		switch os.Args[1] {
		case "install":
			run = func() error { return installService(dataDir(), stateDir()) }
		case "uninstall":
			run = uninstallService
		case "browser":
			run = func() error { return runBrowserCmd(os.Args[2:], os.Stdout) }
		case "idle", "active":
			run = func() error { return runIdleHook(os.Args[1], os.Args[2:], stateDir(), time.Now()) }
		}
		if run != nil {
			if err := run(); err != nil {
				fmt.Fprintf(os.Stderr, "trackd %s: %v\n", os.Args[1], err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	dump := flag.Bool("dump", false, "print live samples to stdout and exit on SIGINT; never touches the database")
	flag.Parse()

	if *dump {
		src, err := newSource()
		if err != nil {
			fmt.Fprintf(os.Stderr, "trackd: %v\n", err)
			os.Exit(1)
		}
		os.Exit(runDump(src, os.Stdout, os.Getppid()))
	}

	// trackd's budget is 40 MB. The live heap is a few MB, but rebuilding
	// the history after a rule change allocates hundreds; a soft limit makes
	// the collector work harder then instead of letting the heap grow.
	debug.SetMemoryLimit(memoryLimit)
	// Between those rebuilds the live heap is about 10 MB; GC at half
	// again over it instead of double keeps the heap's swing small. Each
	// tick's garbage is small (the label memos), so the extra GC is cheap.
	debug.SetGCPercent(gcPercent)
	store.SetCacheKiB(cacheKiB)

	o := daemonOpts{
		DataDir:   dataDir(),
		StateDir:  stateDir(),
		ConfigDir: configDir(),
		NewSource: newSource,
		Stderr:    os.Stderr,
		Notifier:  osascriptNotifier{},
		Remover:   defaultRemover(),
		Tock:      findTock(),
		Camera:    collect.CameraInUse,
		Mic:       collect.MicInUse,
		Calendar:  collect.EventKit{},
		OCR:       newOCRCapturer(),
		FreeEvery: freeEvery,
		// SecureInput is a cheap Carbon call, made only before a read.
		SecureInput: collect.SecureInput,
		// A terminal-launched trackd would attribute the prompt to the
		// terminal (see newSource).
		CalendarPrompt: os.Getppid() == 1,
		SessionEnding:  sessionEnding,
	}
	if exe, err := os.Executable(); err == nil {
		if exe, err = filepath.EvalSymlinks(exe); err == nil {
			o.SnapshotExe = exe
			o.Premigrate = workerMigrate(exe)
		}
	}
	if runtime.GOOS == "windows" {
		// Notices go through the tray app once it exists; until then none.
		o.Notifier, o.Camera, o.Mic, o.Calendar, o.OCR = nil, nil, nil, nil, nil
	}
	if runtime.GOOS == "linux" {
		o.Notifier, o.Camera, o.Mic, o.Calendar, o.OCR = notifySendNotifier{}, nil, nil, nil, nil
		// Self-update replaces the files next to this one, as the unit runs it.
		if exe, err := os.Executable(); err == nil {
			if exe, err = filepath.EvalSymlinks(exe); err == nil {
				o.Exe = exe
			}
		}
	}
	rc := runDaemon(o)
	if rc == exitUpdated {
		// runDaemon has closed the store and let go of the lock. Exec keeps
		// the pid, so systemd sees one process.
		err := execInto(filepath.Join(filepath.Dir(o.Exe), "trackd"))
		fmt.Fprintf(os.Stderr, "trackd: restart into the update: %v\n", err)
		rc = exitRestart // systemd starts the new binary
	}
	os.Exit(rc)
}
