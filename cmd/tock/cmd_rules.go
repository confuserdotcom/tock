package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

// cmdInit writes the starter rules.toml; it never overwrites.
func cmdInit(env Env, args []string) error {
	if len(args) > 0 {
		return usagef("init takes no arguments")
	}
	path := env.rulesPath()
	if err := os.MkdirAll(env.ConfigDir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s already exists; not overwriting", path)
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(classify.Starter); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "wrote %s\n", path)
	return nil
}

const rulesUsage = "usage: tock rules check | tock rules stats [--days N]"

// cmdRules implements "tock rules check" and "tock rules stats".
func cmdRules(env Env, args []string) error {
	switch {
	case len(args) > 0 && args[0] == "stats":
		return cmdRulesStats(env, args[1:])
	case len(args) != 1 || args[0] != "check":
		return usagef(rulesUsage)
	}
	rs, err := classify.Load(env.rulesPath(), env.learnedPath())
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "rules.toml: %d categories, %d projects, %d rules\nlearned.toml: %d rules\n",
		len(rs.Categories), len(rs.Projects), len(rs.Rules), len(rs.Learned))
	if rs.CallCategory != "" {
		fmt.Fprintf(env.Stdout, "calls: labelled %s\n", rs.CallCategory)
	} else {
		fmt.Fprintln(env.Stdout, "calls: not labelled (add [calls] category to rules.toml)")
	}

	st, err := openStore(env)
	if errors.Is(err, errNoDB) {
		return nil
	}
	if err != nil {
		return err
	}
	defer st.Close()
	ovs, err := st.Overrides()
	if err != nil {
		return err
	}
	stale := map[string]int{}
	for _, o := range ovs {
		if o.Category != "" && !rs.HasCategory(o.Category) {
			stale[fmt.Sprintf("category %q", o.Category)]++
		}
		if o.Project != "" && !rs.HasProject(o.Project) {
			stale[fmt.Sprintf("project %q", o.Project)]++
		}
	}
	names := make([]string, 0, len(stale))
	for n := range stale {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(env.Stdout, "warning: %d manual tags name undeclared %s\n", stale[n], n)
	}
	return nil
}

// cmdRulesStats prints, for every rule in check order, the time it decided
// a label over the last days, and flags rules that never matched or never
// decided anything.
func cmdRulesStats(env Env, args []string) error {
	fs := newFlags("rules stats")
	days := fs.Int("days", app.StatsDays, "window in days")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *days < 1 || fs.NArg() > 0 {
		return usagef(rulesUsage)
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	stats, err := a.RuleStats(*days)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "last %d days · time decided\n", *days)
	for _, s := range stats {
		ref := strconv.Itoa(s.Rule.Index)
		if s.Rule.File == classify.LearnedFile {
			ref = "L" + ref
		}
		dur := "0m"
		if s.DecidedMS > 0 {
			dur = report.Dur(s.DecidedMS)
		}
		line := fmt.Sprintf("%-4s %6s  %s  %s", ref, dur,
			report.Pad(report.Safe(report.LabelText(s.Rule.Category, s.Rule.Project)), 20), report.Safe(s.Rule.Describe()))
		switch s.Status() {
		case classify.StatusUnused:
			line += "  unused"
		case classify.StatusShadowed:
			line += "  shadowed (matched " + report.Dur(s.MatchedMS) + ")"
		}
		fmt.Fprintln(env.Stdout, line)
	}
	return nil
}
