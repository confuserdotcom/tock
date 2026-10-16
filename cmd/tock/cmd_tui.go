package main

import (
	"bufio"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/plan"
	"tock/internal/tui"
)

// welcome is shown by `tock` on a terminal when there is no rules.toml yet.
const welcome = `
  tock — where your time goes

  trackd records the app and window in front, all day, on this Mac only.
  tock labels that time with your rules and helps you label the rest.

  You have no rules yet. The starter rules cover common apps and sites,
  and you can change them any time (rules tab, e).

`

// runProgram runs the TUI; tests replace it.
var runProgram = func(env Env, m tui.Model) error {
	_, err := tea.NewProgram(m, tea.WithInput(env.Stdin), tea.WithOutput(env.Stdout)).Run()
	return err
}

// cmdTUI runs the full-screen interface. Startup errors (no rules, no
// database) are returned before the screen changes; with no rules it first
// offers to write the starter rules.
func cmdTUI(env Env) error { return cmdTUIOn(env, "") }

// cmdTUIOn is cmdTUI opened on a named tab ("" = today).
func cmdTUIOn(env Env, tab string) error {
	a, err := openApp(env)
	if errors.Is(err, classify.ErrNoRules) {
		fmt.Fprint(env.Stdout, welcome)
		fmt.Fprintf(env.Stdout, "  Create starter rules in %s? [Y/n] ", env.rulesPath())
		answer, _ := bufio.NewReader(env.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "" && a != "y" && a != "yes" {
			fmt.Fprintln(env.Stdout, "\n  No rules written. Run tock init when you are ready.")
			return nil
		}
		if err := cmdInit(env, nil); err != nil {
			return err
		}
		a, err = openApp(env)
	}
	if err != nil {
		return err
	}
	defer a.Close()
	m := tui.New(a, env.Now).WithTab(tab)
	if env.APIKey != "" {
		m = m.WithSuggester(env.NewClient(env.APIKey))
	}
	return runProgram(env, m)
}

// cmdPlan opens the TUI on the plan tab; not on a terminal it prints the
// plan's open tasks, one per line, by section.
func cmdPlan(env Env, args []string) error {
	if len(args) > 0 {
		return usagef("plan takes no arguments")
	}
	if env.Interactive {
		return cmdTUIOn(env, "plan")
	}
	c, err := config.Load(filepath.Join(env.ConfigDir, config.File))
	if err != nil {
		return err
	}
	if c.Plan.File == "" {
		return errors.New("no plan file: set [plan] file in config.toml")
	}
	lines, err := plan.Read(c.Plan.File)
	if err != nil {
		return err
	}
	now := env.Now()
	sec := -1
	for _, r := range plan.Rows(lines, now.Format("2006-01-02"), now.AddDate(0, 0, 1).Format("2006-01-02")) {
		if r.Done {
			continue
		}
		if r.Section != sec {
			sec = r.Section
			fmt.Fprintln(env.Stdout, plan.SectionNames[sec])
		}
		fmt.Fprintln(env.Stdout, "  "+r.Raw)
	}
	return nil
}
