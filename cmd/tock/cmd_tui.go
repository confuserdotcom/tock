package main

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"tock/internal/classify"
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
func cmdTUI(env Env) error {
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
	m := tui.New(a, env.Now)
	if env.APIKey != "" {
		m = m.WithSuggester(env.NewClient(env.APIKey))
	}
	return runProgram(env, m)
}
