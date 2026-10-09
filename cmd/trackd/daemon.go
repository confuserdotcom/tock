package main

import (
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"tock/internal/collect"
	"tock/internal/span"
	"tock/internal/store"
)

type daemonOpts struct {
	DataDir, StateDir string
	NewSource         func() (collect.Source, error)
	Stderr            io.Writer
}

// runDaemon wires source, merger and store. It must be called on the main
// goroutine because src.Run does.
func runDaemon(o daemonOpts) int {
	log := slog.New(slog.NewTextHandler(o.Stderr, nil))
	dbPath := filepath.Join(o.DataDir, "tock.db")
	log.Info("trackd starting", "version", version, "db", dbPath)

	st, err := store.Open(dbPath)
	if err != nil {
		log.Error("open store", "err", err)
		return 1
	}
	next, err := st.NextID()
	if err != nil {
		log.Error("next id", "err", err)
		_ = st.Close()
		return 1
	}
	cfg := span.DefaultConfig()
	cfg.NextID = next
	m := span.New(cfg)

	src, err := o.NewSource()
	if err != nil {
		log.Error("create source", "err", err)
		_ = st.Close()
		return 1
	}

	pipeErr := make(chan error, 1)
	go func() {
		err := pipeline(src.Inputs(), m, st, nil)
		if err != nil {
			src.Stop() // a dead pipeline must not leave the run loop spinning
		}
		pipeErr <- err
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		log.Info("signal received, stopping")
		src.Stop()
	}()

	if err := src.Run(); err != nil {
		log.Error("source run", "err", err)
	}
	rc := 0
	if err := <-pipeErr; err != nil {
		log.Error("pipeline", "err", err)
		rc = 1
	}
	if err := st.Close(); err != nil {
		log.Error("close store", "err", err)
		rc = 1
	}
	return rc
}

// pipeline feeds every input to the merger and applies the resulting ops.
// When in closes it flushes the merger. A nil tick channel is allowed.
func pipeline(in <-chan span.Input, m *span.Merger, st *store.Store, tick <-chan time.Time) error {
	for {
		select {
		case x, ok := <-in:
			if !ok {
				return st.Apply(m.Flush())
			}
			if err := st.Apply(m.Feed(x)); err != nil {
				return err
			}
		case <-tick:
			if err := st.Apply(m.Tick(time.Now().Round(0).UnixMilli())); err != nil {
				return err
			}
		}
	}
}
