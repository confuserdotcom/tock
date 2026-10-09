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
func runDump(src collect.Source, w io.Writer) int {
	if src == nil {
		return 1
	}
	fmt.Fprintf(w, "# trackd dump version=%s pid=%d\n", version, os.Getpid())

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
		auto := s.Automation
		if auto == "" {
			auto = "-"
		}
		return fmt.Sprintf("%s sample bundle=%s app=%q title=%s url=%s private=%d idle=%.1f locked=%d ax=%d auto=%s act=%d",
			ts, s.BundleID, s.App, quoteOrDash(s.Title), quoteOrDash(s.URL),
			b2i(s.Private), s.IdleS, b2i(s.Locked), b2i(s.AXTrusted), auto, b2i(s.Activation))
	case in.Event != nil:
		e := in.Event
		return fmt.Sprintf("%s event %s", time.UnixMilli(e.TS).Local().Format(dumpTSLayout), e.Kind)
	}
	return ""
}
