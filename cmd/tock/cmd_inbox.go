package main

import (
	"fmt"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/store"
)

const inboxTop = 20

func cmdInbox(env Env, args []string) error {
	fs := newFlags("inbox")
	days := fs.Int("days", 7, "how many days back to look")
	all := fs.Bool("all", false, "show every key, not only the top 20 above [inbox] min_minutes")
	auto := fs.Bool("auto", false, "list the keys tock labelled on its own")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 || *days < 1 {
		return usagef("inbox: usage: tock inbox [--days N] [--all | --auto]")
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	if *auto {
		return listAuto(env, a, *days)
	}
	if _, err := a.AutoAccept(*days); err != nil {
		return err
	}
	today, err := a.AutoLabels(midnightMS(env.Now()))
	if err != nil {
		return err
	}
	items, err := a.Inbox(*days)
	if err != nil {
		return err
	}
	sugg, err := a.Suggestions()
	if err != nil {
		return err
	}
	guesses, err := a.Guesses()
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintf(env.Stdout, "Inbox empty: everything in the last %d days is labelled.\n", *days)
		autoLine(env, today)
		return saveListing(env, nil)
	}
	var small []classify.Item
	if !*all {
		items, small = classify.Floor(items, a.Config().Inbox.MinMS)
	}
	var total int64
	for _, it := range items {
		total += it.TotalMS
	}
	fmt.Fprintf(env.Stdout, "Inbox  last %d days · %s · %s unlabelled\n", *days, plural(len(items), "key"), report.Dur(total))
	shown := items
	if !*all && len(shown) > inboxTop {
		shown = shown[:inboxTop]
	}
	keys := make([]string, len(shown))
	for i, it := range shown {
		keys[i] = it.Key
		line := fmt.Sprintf("%3d  %5s  %s", i+1, report.Dur(it.TotalMS), report.Safe(it.Key))
		if it.Pages > 1 {
			line += fmt.Sprintf(" · %d pages", it.Pages)
		}
		if it.Example != "" {
			line += fmt.Sprintf("  %q", it.Example)
		}
		if w, ok := sugg[it.Key]; ok {
			line += "  → " + tagText(w)
			if g, ok := guesses[it.Key]; ok && g.Want == w {
				line += fmt.Sprintf(" (%s, %d%%)", g.Reason, int(g.Share*100+0.5))
			}
		}
		fmt.Fprintln(env.Stdout, line)
	}
	if len(shown) < len(items) {
		fmt.Fprintf(env.Stdout, "… %d more (tock inbox --all)\n", len(items)-len(shown))
	}
	if len(small) > 0 {
		var ms int64
		for _, it := range small {
			ms += it.TotalMS
		}
		fmt.Fprintf(env.Stdout, "+ %s under %s (%s) · tock inbox --all\n",
			plural(len(small), "small key"), report.Dur(a.Config().Inbox.MinMS), report.Dur(ms))
	}
	autoLine(env, today)
	fmt.Fprintln(env.Stdout, "tag: tock tag <n> <Category>[/<Project>]   accept →: tock tag <n>   hide: tock ignore <n>")
	return saveListing(env, keys)
}

// autoLine says how many keys tock labelled on its own today.
func autoLine(env Env, today []store.AutoLabel) {
	if len(today) == 0 {
		return
	}
	var ms int64
	for _, x := range today {
		ms += x.MS
	}
	fmt.Fprintf(env.Stdout, "auto-labelled %s (%s) today · tock inbox --auto\n", plural(len(today), "key"), report.Dur(ms))
}

// listAuto is tock inbox --auto: the keys auto-labelled in the last days
// days, numbered so tock tag can relabel them.
func listAuto(env Env, a *app.App, days int) error {
	xs, err := a.AutoLabels(env.Now().AddDate(0, 0, -days).UnixMilli())
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "Auto-labelled  last %d days · %s\n", days, plural(len(xs), "key"))
	keys := make([]string, len(xs))
	for i, x := range xs {
		keys[i] = x.Key
		fmt.Fprintf(env.Stdout, "%3d  %5s  %s  → %s (%s, %d%%)\n", i+1, report.Dur(x.MS), report.Safe(x.Key),
			tagText(classify.Want{Category: x.Category, Project: x.Project}), x.Reason, int(x.Share*100+0.5))
	}
	if len(xs) > 0 {
		fmt.Fprintln(env.Stdout, "wrong? tock tag <n> <Category>[/<Project>] relabels it and replaces the rule")
	}
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

// tagText is a label in `tock tag` syntax: "Category", "Category/Project"
// or "/Project".
func tagText(w classify.Want) string {
	if w.Project == "" {
		return w.Category
	}
	return w.Category + "/" + w.Project
}

// midnightMS is the start of t's local day.
func midnightMS(t time.Time) int64 {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).UnixMilli()
}
