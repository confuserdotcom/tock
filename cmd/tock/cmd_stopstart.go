package main

import "fmt"

// cmdStop stops the background services (trackd, and the menu bar on
// macOS) until tock start or the next login.
func cmdStop(env Env, args []string) error {
	fs := newFlags("stop")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("stop: usage: tock stop")
	}
	if err := restoreStop(env); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	fmt.Fprintln(env.Stdout, "stopped; recording resumes with tock start or at your next login")
	return nil
}

// cmdStart starts the background services again.
func cmdStart(env Env, args []string) error {
	fs := newFlags("start")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("start: usage: tock start")
	}
	if err := restoreStart(env); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	fmt.Fprintln(env.Stdout, "started")
	return nil
}
