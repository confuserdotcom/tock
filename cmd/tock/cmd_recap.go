package main

import (
	"fmt"
	"time"
)

// cmdRecap prints the daily recap of today or of a YYYY-MM-DD date, or with
// "start" the text the start-of-day notice would carry now. The preview only
// reads: it never marks the notice as sent.
func cmdRecap(env Env, args []string) error {
	if len(args) > 0 && args[0] == "start" {
		if len(args) > 1 {
			return usagef("recap start takes no arguments")
		}
		a, err := openApp(env)
		if err != nil {
			return err
		}
		defer a.Close()
		text, err := a.StartBody(env.Now())
		if err != nil {
			return err
		}
		fmt.Fprintln(env.Stdout, text)
		return nil
	}
	if len(args) > 1 {
		return usagef("recap takes at most one date")
	}
	day := env.Now()
	if len(args) == 1 {
		d, err := time.ParseInLocation("2006-01-02", args[0], env.Now().Location())
		if err != nil {
			return usagef("recap: date %q, want YYYY-MM-DD", args[0])
		}
		day = d
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	text, err := a.Recap(day)
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, text)
	return nil
}
