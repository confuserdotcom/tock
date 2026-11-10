package main

import (
	"fmt"

	"tock/internal/app"
)

// cmdStatus prints one line for a status bar.
func cmdStatus(env Env, args []string) error {
	if len(args) > 0 {
		return usagef("status takes no arguments")
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	line, err := a.Status()
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, line)
	// The status line stays one line for prompts; the fix goes to stderr.
	if h, err := a.Health(); err == nil && h.TooOld() {
		fmt.Fprintf(env.Stderr, "%s; update it: %s\n", app.TooOldText, installMethod().Reinstall(env.Linux))
	}
	return nil
}
