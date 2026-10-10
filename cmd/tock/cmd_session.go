package main

import (
	"fmt"
	"strconv"
	"time"

	"tock/internal/config"
	"tock/internal/report"
)

// cmdSession shows, starts or stops the focus session.
func cmdSession(env Env, args []string) error {
	if len(args) == 0 {
		return sessionShow(env)
	}
	switch args[0] {
	case "start":
		return sessionStart(env, args[1:])
	case "stop":
		if len(args) > 1 {
			return usagef("session stop takes no arguments")
		}
		return sessionStop(env)
	}
	return usagef("session: unknown subcommand %q", args[0])
}

func sessionShow(env Env) error {
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	f, err := a.Focus(env.Now())
	if err != nil {
		return err
	}
	if f.Open == nil {
		fmt.Fprintln(env.Stdout, "no session")
		return nil
	}
	fmt.Fprintln(env.Stdout, "focus "+report.SessionStatus(*f.Open, f.NowMS))
	return nil
}

func sessionStart(env Env, args []string) error {
	if len(args) > 1 {
		return usagef("session start takes one length in minutes")
	}
	n := 0
	if len(args) == 1 {
		v, err := strconv.Atoi(args[0])
		if err != nil || v < 1 || v > config.MaxSessionMin {
			return usagef("session start: length %q, want 1 to %d minutes", args[0], config.MaxSessionMin)
		}
		n = v
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	if n == 0 {
		n = a.Config().Focus.Presets[0]
	}
	s, err := a.StartSession(n)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "session started: %d min, ends %s\n", n,
		time.UnixMilli(s.PlannedMS).In(env.Now().Location()).Format("15:04"))
	if h, err := a.Health(); err == nil && !h.TrackdUp {
		fmt.Fprintln(env.Stderr, "warning: trackd not running, no alerts")
	}
	return nil
}

func sessionStop(env Env) error {
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	s, err := a.StopSession()
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, "stopped: "+report.SessionLine(s, env.Now().Location(), false))
	return nil
}
