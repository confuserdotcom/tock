package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tock/internal/collect"
	"tock/internal/span"
)

const dumpTSLayout = "2006-01-02T15:04:05.000-07:00"

// runDump prints live inputs to w until interrupted. It opens no database and
// takes no lock (single writer, D-22): there is no DB handle in dump mode.
// A nil src means NewDarwin failed; the caller reports the error.
func runDump(src collect.Source, w io.Writer, ppid int) int {
	if src == nil {
		return 1
	}
	writeDumpBanner(w, ppid)

	printed := make(chan struct{})
	go func() {
		defer close(printed)
		for in := range src.Inputs() {
			fmt.Fprintln(w, formatInput(in))
		}
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		src.Stop()
	}()

	if err := src.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "trackd: source: %v\n", err)
	}
	<-printed
	return 0
}

// writeDumpBanner prints the banner and, when not started by launchd or
// open, a notice that macOS attributes the permissions to the terminal
// (RESEARCH Pitfall 2).
func writeDumpBanner(w io.Writer, ppid int) {
	fmt.Fprintf(w, "# trackd dump version=%s pid=%d ppid=%d\n", version, os.Getpid(), ppid)
	if ppid != 1 {
		fmt.Fprintln(w, "# permissions shown are those of the launching terminal, not the com.tock.trackd agent; run 'make dump' for the agent's view")
	}
}

func quoteOrDash(s *string) string {
	if s == nil {
		return "-"
	}
	return fmt.Sprintf("%q", *s)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// formatInput renders one dump line. The field names are a contract that
// later plans grep for, so keep them byte-exact.
func formatInput(in span.Input) string {
	switch {
	case in.Sample != nil:
		s := in.Sample
		ts := time.UnixMilli(s.TS).Local().Format(dumpTSLayout)
		title, url := s.Title, s.URL
		if s.Private {
			// Defense in depth: never print private data even if an upstream
			// bug left it on the sample (T-1-private).
			title, url = nil, nil
		}
		auto := s.Automation
		if auto == "" {
			auto = "-"
		}
		return fmt.Sprintf("%s sample bundle=%s app=%q title=%s url=%s private=%d idle=%.1f locked=%d ax=%d auto=%s act=%d",
			ts, s.BundleID, s.App, quoteOrDash(title), quoteOrDash(url),
			b2i(s.Private), s.IdleS, b2i(s.Locked), b2i(s.AXTrusted), auto, b2i(s.Activation))
	case in.Event != nil:
		e := in.Event
		return fmt.Sprintf("%s event %s", time.UnixMilli(e.TS).Local().Format(dumpTSLayout), e.Kind)
	}
	return ""
}
