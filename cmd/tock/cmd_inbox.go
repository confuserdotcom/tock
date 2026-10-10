package main

import (
	"fmt"

	"tock/internal/report"
)

const inboxTop = 20

func cmdInbox(env Env, args []string) error {
	fs := newFlags("inbox")
	days := fs.Int("days", 7, "how many days back to look")
	all := fs.Bool("all", false, "show every key, not only the top 20")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 || *days < 1 {
		return usagef("inbox: usage: tock inbox [--days N] [--all]")
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	items, err := a.Inbox(*days)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintf(env.Stdout, "Inbox empty: everything in the last %d days is labelled.\n", *days)
		return saveListing(env, nil)
	}
	var total int64
	for _, it := range items {
		total += it.TotalMS
	}
	fmt.Fprintf(env.Stdout, "Inbox  last %d days · %d keys · %s unlabelled\n", *days, len(items), report.Dur(total))
	shown := items
	if !*all && len(shown) > inboxTop {
		shown = shown[:inboxTop]
	}
	keys := make([]string, len(shown))
	for i, it := range shown {
		keys[i] = it.Key
		line := fmt.Sprintf("%3d  %5s  %s", i+1, report.Dur(it.TotalMS), report.Safe(it.Key))
		if it.Example != "" {
			line += fmt.Sprintf("  %q", it.Example)
		}
		fmt.Fprintln(env.Stdout, line)
	}
	if len(shown) < len(items) {
		fmt.Fprintf(env.Stdout, "… %d more (tock inbox --all)\n", len(items)-len(shown))
	}
	fmt.Fprintln(env.Stdout, "tag: tock tag <n> <Category>[/<Project>]   hide: tock ignore <n>")
	return saveListing(env, keys)
}

func cmdIgnore(env Env, args []string) error {
	fs := newFlags("ignore")
	undo := fs.Bool("undo", false, "show a hidden key again (pass the key text)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usagef("ignore: usage: tock ignore <n> | tock ignore --undo <key>")
	}
	key := fs.Arg(0)
	if !*undo {
		var err error
		if key, err = resolveListed(env, key); err != nil {
			return err
		}
	}
	st, err := openStore(env)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.SetIgnored(key, !*undo, env.Now().UnixMilli()); err != nil {
		return err
	}
	if *undo {
		fmt.Fprintf(env.Stdout, "showing again: %s\n", report.Safe(key))
	} else {
		fmt.Fprintf(env.Stdout, "ignored: %s (undo: tock ignore --undo %q)\n", report.Safe(key), key)
	}
	return nil
}
