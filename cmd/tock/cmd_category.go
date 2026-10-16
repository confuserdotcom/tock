package main

import (
	"fmt"
	"strings"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

const categoryUsage = `usage: tock category                          list categories, their projects and 30 days of time
       tock category add <name> <kind> [--afk D]  kind: deep, shallow, neutral, browse or distract
       tock category set <name> [--kind K] [--afk D|none]
       tock category rename <name> <new name>
       tock category rm <name>                    only when nothing names it (else: tock rules merge)`

// cmdCategory manages the categories in rules.toml.
func cmdCategory(env Env, args []string) error {
	if len(args) == 0 {
		return cmdCategoryList(env)
	}
	sub, rest := args[0], args[1:]
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	switch sub {
	case "add":
		fs := newFlags("category add")
		afk := fs.String("afk", "", "AFK window, 5m to 2h")
		if err := parseFlags(fs, moveFlagsFirst(rest)); err != nil {
			return err
		}
		if fs.NArg() != 2 {
			return usagef(categoryUsage)
		}
		ms, err := afkFlag(*afk)
		if err != nil {
			return err
		}
		if err := a.AddCategory(app.CategorySpec{Name: fs.Arg(0), Kind: fs.Arg(1), AFKMS: ms}); err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "added category %s (%s)\n", report.Safe(strings.TrimSpace(fs.Arg(0))), fs.Arg(1))
	case "set":
		fs := newFlags("category set")
		kind := fs.String("kind", "", "deep, shallow, neutral, browse or distract")
		afk := fs.String("afk", "", "AFK window, 5m to 2h, or none")
		if err := parseFlags(fs, moveFlagsFirst(rest)); err != nil {
			return err
		}
		if fs.NArg() != 1 || (*kind == "" && *afk == "") {
			return usagef(categoryUsage)
		}
		ms, err := afkFlag(*afk)
		if err != nil {
			return err
		}
		name, err := a.Rules().ResolveCategory(fs.Arg(0))
		if err != nil {
			return err
		}
		if err := a.EditCategory(app.CategorySpec{Name: name, Kind: *kind, AFKMS: ms}); err != nil {
			return err
		}
		c := categoryOf(a.Rules(), name)
		fmt.Fprintf(env.Stdout, "%s: %s, afk %s\n", report.Safe(name), c.Kind, classify.FormatDur(a.Rules().AFKWindow(name)))
	case "rename":
		if len(rest) != 2 {
			return usagef(categoryUsage)
		}
		from, err := a.RenameName(classify.NameCategory, rest[0], rest[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "renamed category %s to %s everywhere (old rules.toml kept as rules.toml.bak)\n",
			report.Safe(from), report.Safe(strings.TrimSpace(rest[1])))
	case "rm", "remove":
		if len(rest) != 1 {
			return usagef(categoryUsage)
		}
		name, err := a.RemoveName(classify.NameCategory, rest[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "removed category %s (old rules.toml kept as rules.toml.bak)\n", report.Safe(name))
	default:
		return usagef(categoryUsage)
	}
	return nil
}

func cmdCategoryList(env Env) error {
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	tree, err := a.LabelTree(30)
	if err != nil {
		return err
	}
	w := 0
	for _, c := range tree {
		w = max(w, len([]rune(c.Name)))
	}
	for _, c := range tree {
		name := c.Name
		if name == "" {
			fmt.Fprintln(env.Stdout, "no category")
		} else {
			afk := ""
			if c.AFKMS > 0 {
				afk = "  afk " + classify.FormatDur(c.AFKMS)
			}
			fmt.Fprintf(env.Stdout, "%-*s  %-8s  %6s%s\n", w, report.Safe(name), c.Kind, usedText(c.MS), afk)
		}
		for _, p := range c.Projects {
			arch := ""
			if p.Archived {
				arch = "  archived"
			}
			fmt.Fprintf(env.Stdout, "  %-*s  %6s%s\n", w+8, report.Safe(p.Name), usedText(p.MS), arch)
		}
	}
	return nil
}

func categoryOf(rs *classify.Ruleset, name string) classify.Category {
	for _, c := range rs.Categories {
		if strings.EqualFold(c.Name, name) {
			return c
		}
	}
	return classify.Category{}
}

// afkFlag reads --afk: "" leaves it, "none" drops it, else a duration.
func afkFlag(s string) (int64, error) {
	switch s {
	case "":
		return 0, nil
	case "none", "off", "default":
		return -1, nil
	}
	ms, err := classify.ParseDur(s)
	if err != nil {
		return 0, usagef("--afk: %v", err)
	}
	return ms, nil
}

// moveFlagsFirst lets flags follow the arguments: `add Reading neutral
// --afk 10m`. Every flag here takes a value.
func moveFlagsFirst(args []string) []string {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") && len(args[i]) > 1 {
			flags = append(flags, args[i])
			if !strings.Contains(args[i], "=") && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		rest = append(rest, args[i])
	}
	return append(flags, rest...)
}
