package main

import (
	"fmt"
	"time"

	"tock/internal/app"
)

// maxSyncDays bounds tock sync --days: a year of day files.
const maxSyncDays = 366

// cmdSync runs one sync now (trackd does it every minute) and lists the
// other machines. --days writes older days, for the first sync.
func cmdSync(env Env, args []string) error {
	fs := newFlags("sync")
	days := fs.Int("days", app.SyncDays, "days to write, today included")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("sync takes only flags")
	}
	if *days < 1 || *days > maxSyncDays {
		return usagef("sync: --days %d, want 1 to %d", *days, maxSyncDays)
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	r, err := a.Sync(*days)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "%s: wrote %d day files, read %d\n", r.Machine, r.Written, r.Read)
	if r.RulesFrom != "" {
		fmt.Fprintf(env.Stdout, "took the newer rules.toml from %s (yours is rules.toml.bak)\n", r.RulesFrom)
	}
	if r.RulesRefused != "" {
		fmt.Fprintf(env.Stdout, "kept your rules.toml: the newer one from %s\n", r.RulesRefused)
	}
	if len(r.Machines) == 0 {
		fmt.Fprintln(env.Stdout, "no other machine yet")
	}
	for _, m := range r.Machines {
		last := "no activity"
		if m.LastMS > 0 {
			last = "last active " + time.UnixMilli(m.LastMS).In(env.Now().Location()).Format("Mon 2 Jan 15:04")
		}
		fmt.Fprintf(env.Stdout, "%s: %s (%s)\n", m.Name, last, plural(m.Days, "day"))
	}
	return nil
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
