package main

import (
	"errors"
	"fmt"

	"tock/internal/app"
	"tock/internal/config"
)

// cmdPrune deletes old window titles now.
func cmdPrune(env Env, args []string) error {
	fs := newFlags("prune")
	days := fs.Int("days", 0, "keep this many days of titles (default: [privacy] keep_titles)")
	dry := fs.Bool("dry-run", false, "only count what would be deleted")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("prune: unexpected argument %q", fs.Arg(0))
	}
	if *days < 0 || *days > config.MaxKeepTitles {
		return usagef("prune: --days %d, want 1 to %d", *days, config.MaxKeepTitles)
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	r, err := a.PruneTitles(*days, *dry)
	if errors.Is(err, app.ErrPruneOff) {
		return usagef("prune: %v, or pass --days N", err)
	}
	if err != nil {
		return err
	}
	verb := "deleted"
	if *dry {
		verb = "would delete"
	}
	line := fmt.Sprintf("%s the titles of %s before %s", verb, plural(r.Spans, "span"), r.Before.Format("2006-01-02"))
	if r.Remote > 0 {
		line += fmt.Sprintf(", %s from other machines", plural(r.Remote, "view"))
	}
	if r.Files > 0 {
		line += fmt.Sprintf(", %s in the sync folder", plural(r.Files, "day file"))
	}
	fmt.Fprintln(env.Stdout, line)
	return nil
}
