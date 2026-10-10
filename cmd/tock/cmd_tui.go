package main

import (
	tea "charm.land/bubbletea/v2"

	"tock/internal/tui"
)

// cmdTUI runs the full-screen interface. Startup errors (no rules, no
// database) are returned before the screen changes.
func cmdTUI(env Env) error {
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	_, err = tea.NewProgram(tui.New(a, env.Now), tea.WithInput(env.Stdin), tea.WithOutput(env.Stdout)).Run()
	return err
}
