package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"tock/internal/web"
)

// cmdWeb serves the dashboard on the loopback interface until interrupted.
// The link it prints carries the per-run token in the URL fragment.
func cmdWeb(env Env, args []string) error {
	fs := newFlags("web")
	port := fs.Int("port", 0, "port on 127.0.0.1 (default: any free port)")
	noOpen := fs.Bool("no-open", false, "print the link without opening the browser")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 || *port < 0 || *port > 65535 {
		return usagef("web: usage: tock web [--port N] [--no-open]")
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	token, err := web.NewToken()
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		return err
	}
	p := ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{
		Handler:           web.New(a, token, p, env.Now).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	link := fmt.Sprintf("http://127.0.0.1:%d/#%s", p, token)
	fmt.Fprintf(env.Stdout, "tock web: %s\nOnly this machine can connect. Press ctrl-c to stop.\n", link)
	if !*noOpen && env.Open != nil {
		if err := env.Open(link); err != nil {
			fmt.Fprintf(env.Stderr, "tock: cannot open the browser: %v\n", err)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}
	return nil
}
