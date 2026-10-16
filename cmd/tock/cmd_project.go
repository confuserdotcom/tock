package main

import (
	"fmt"
	"sort"
	"strings"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

const projectUsage = `usage: tock project                                    categories with their projects (same as tock category)
       tock project add <name> [--category C] [--alias a,b]   with a category it is a module
       tock project set <name> [--category C|none] [--alias a,b|none] [--archived true|false]
       tock project rename <name> <new name>
       tock project rm <name>                                only when nothing names it
       tock project scan <dir> --category C                  a module for each folder in dir
       tock project refile <name>                            hand tags of a catch-all project go to the module their title names`

// cmdProject manages the projects (and modules) in rules.toml.
func cmdProject(env Env, args []string) error {
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
	case "add", "set":
		fs := newFlags("project " + sub)
		cat := fs.String("category", "", "file it under this category (none: unfile)")
		alias := fs.String("alias", "", "other names, comma-separated (none: drop them)")
		arch := fs.String("archived", "", "true or false")
		if err := parseFlags(fs, moveFlagsFirst(rest)); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return usagef(projectUsage)
		}
		p := app.ProjectSpec{Name: fs.Arg(0)}
		if *cat != "" {
			c := *cat
			if c == "none" {
				c = ""
			}
			p.Category = &c
		}
		if *alias != "" {
			var als []string
			if *alias != "none" {
				als = strings.Split(*alias, ",")
			}
			p.Aliases = &als
		}
		switch *arch {
		case "":
		case "true", "false":
			b := *arch == "true"
			p.Archived = &b
		default:
			return usagef("--archived: true or false")
		}
		if sub == "add" {
			if p.Archived != nil {
				return usagef(projectUsage)
			}
			if err := a.AddProject(p); err != nil {
				return err
			}
			fmt.Fprintf(env.Stdout, "added project %s%s\n", report.Safe(strings.TrimSpace(p.Name)), moduleNote(a, p.Name))
			return nil
		}
		if p.Category == nil && p.Aliases == nil && p.Archived == nil {
			return usagef(projectUsage)
		}
		name, err := a.Rules().ResolveName(classify.NameProject, p.Name)
		if err != nil {
			return err
		}
		if err := a.EditProject(p); err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "%s%s\n", report.Safe(name), moduleNote(a, name))
	case "rename":
		if len(rest) != 2 {
			return usagef(projectUsage)
		}
		from, err := a.RenameName(classify.NameProject, rest[0], rest[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "renamed project %s to %s everywhere (old rules.toml kept as rules.toml.bak)\n",
			report.Safe(from), report.Safe(strings.TrimSpace(rest[1])))
	case "rm", "remove":
		if len(rest) != 1 {
			return usagef(projectUsage)
		}
		name, err := a.RemoveName(classify.NameProject, rest[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "removed project %s (old rules.toml kept as rules.toml.bak)\n", report.Safe(name))
	case "refile":
		if len(rest) != 1 {
			return usagef(projectUsage)
		}
		moved, err := a.RefileProject(rest[0])
		if err != nil {
			return err
		}
		if len(moved) == 0 {
			fmt.Fprintln(env.Stdout, "no hand-tagged span names a module in its title")
			return nil
		}
		names := make([]string, 0, len(moved))
		for p := range moved {
			names = append(names, p)
		}
		sort.Strings(names)
		var parts []string
		total := 0
		for _, p := range names {
			parts = append(parts, fmt.Sprintf("%s %d", report.Safe(p), moved[p]))
			total += moved[p]
		}
		fmt.Fprintf(env.Stdout, "moved %s to modules: %s\n", plural(total, "span"), strings.Join(parts, ", "))
	case "scan":
		fs := newFlags("project scan")
		cat := fs.String("category", "", "the category the modules go under")
		if err := parseFlags(fs, moveFlagsFirst(rest)); err != nil {
			return err
		}
		if fs.NArg() != 1 || *cat == "" {
			return usagef(projectUsage)
		}
		added, err := a.ScanModules(fs.Arg(0), *cat)
		if err != nil {
			return err
		}
		if len(added) == 0 {
			fmt.Fprintln(env.Stdout, "every folder already has a project")
			return nil
		}
		for i := range added {
			added[i] = report.Safe(added[i])
		}
		fmt.Fprintf(env.Stdout, "added %s: %s\n", plural(len(added), "module"), strings.Join(added, ", "))
	default:
		return usagef(projectUsage)
	}
	return nil
}

// moduleNote says what makes a project a module, if it is one.
func moduleNote(a *app.App, name string) string {
	rs := a.Rules()
	for _, p := range rs.Projects {
		if !strings.EqualFold(p.Name, strings.TrimSpace(name)) {
			continue
		}
		var parts []string
		if p.Category != "" {
			parts = append(parts, "module of "+report.Safe(p.Category))
		}
		if len(p.Aliases) > 0 {
			parts = append(parts, "also "+report.Safe(strings.Join(p.Aliases, ", ")))
		}
		if p.Archived {
			parts = append(parts, "archived")
		}
		if len(parts) > 0 {
			return " (" + strings.Join(parts, "; ") + ")"
		}
	}
	return ""
}
