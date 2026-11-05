package main

import (
	"fmt"
	"strconv"
	"time"
)

// breakDefaultMin and breakMaxMin bound tock break's length.
const (
	breakDefaultMin = 5
	breakMaxMin     = 120
)

// cmdBreak is tock break [MIN]: take a break now, as the Take a break button
// does. Break over comes at its end, and the break screen if it runs over.
func cmdBreak(env Env, args []string) error {
	if len(args) > 1 {
		return usagef("break takes one length in minutes")
	}
	n := breakDefaultMin
	if len(args) == 1 {
		v, err := strconv.Atoi(args[0])
		if err != nil || v < 1 || v > breakMaxMin {
			return usagef("break: length %q, want 1 to %d minutes", args[0], breakMaxMin)
		}
		n = v
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	until, err := a.TakeBreak(int64(n) * 60_000)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "break: %d min, until %s\n", n, time.UnixMilli(until).In(env.Now().Location()).Format("15:04"))
	return nil
}
