package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tock/internal/config"
	"tock/internal/tui"
)

// choiceFile keeps tock choice's own answers: "always open the dashboard".
const choiceFile = "choice.json"

// dashIdle is how long a dashboard tock choice starts runs unused.
const dashIdle = "30m"

// dashPoll and dashTries bound the wait for tock web's link: 10 s.
const (
	dashPoll  = 100 * time.Millisecond
	dashTries = 100
)

// cmdChoice is tock choice: on a terminal the list of features to turn on
// and off; piped, the features one per line; tock choice <id> on|off sets
// one.
func cmdChoice(env Env, args []string) error {
	switch {
	case len(args) == 2:
		f := config.FeatureByID(args[0])
		if f == nil {
			return usagef("choice: no feature %q (tock choice lists them)", args[0])
		}
		var v bool
		switch args[1] {
		case "on":
			v = true
		case "off":
		default:
			return usagef("choice: %s takes on or off", f.ID)
		}
		if err := saveFeatures(env, map[string]bool{f.ID: v}); err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "%s: %s\n", f.Name, args[1])
		afterSave(env, map[string]bool{f.ID: v})
		return nil
	case len(args) != 0:
		return usagef("choice: usage: tock choice [<feature> on|off]")
	}
	cfg, err := config.Load(env.configPath())
	if err != nil {
		return err
	}
	if !env.Interactive {
		for _, f := range config.Features {
			fmt.Fprintf(env.Stdout, "%-4s %-13s %s\n", onOffWord(f.On(cfg)), f.ID, f.Name)
		}
		return nil
	}
	m := tui.NewChoice(cfg, choiceAlways(env), tui.ChoiceDeps{
		Save:     func(c map[string]bool) error { return saveFeatures(env, c) },
		Open:     func() (string, error) { return openDashboard(env) },
		Remember: func() error { return rememberAlways(env) },
	})
	m, err = runChoice(env, m)
	if err != nil {
		return err
	}
	if n := m.Pending(); n > 0 {
		fmt.Fprintf(env.Stdout, "%d change(s) not saved\n", n)
	}
	afterSave(env, m.Saved())
	return nil
}

// runChoice runs the list; tests replace it.
var runChoice = func(env Env, m tui.Choice) (tui.Choice, error) {
	out, err := tea.NewProgram(m, tea.WithInput(env.Stdin), tea.WithOutput(env.Stdout)).Run()
	if c, ok := out.(tui.Choice); ok {
		m = c
	}
	return m, err
}

func (e Env) configPath() string { return filepath.Join(e.ConfigDir, config.File) }

func onOffWord(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

// saveFeatures writes changes into config.toml; see config.SaveFeatures.
func saveFeatures(env Env, changes map[string]bool) error {
	return config.SaveFeatures(env.configPath(), changes)
}

// afterSave does what a feature needs once it is on: the Screen Recording
// grant for screen text, the Slack login for Slack.
func afterSave(env Env, saved map[string]bool) {
	if saved["ocr"] && !env.Linux {
		fmt.Fprintln(env.Stdout, "Screen text needs Screen Recording for trackd; asking…")
		if err := ocrGrant(env); err != nil {
			fmt.Fprintf(env.Stdout, "%v\nthen run tock ocr grant\n", err)
		}
	}
	if saved["slack"] && env.slackToken() == "" {
		fmt.Fprintln(env.Stdout, "Slack needs a token: run tock slack login")
	}
}

type choiceState struct {
	OpenDashboard string `json:"open_dashboard,omitempty"` // "always"
}

func choiceAlways(env Env) bool {
	b, err := os.ReadFile(filepath.Join(env.StateDir, choiceFile))
	if err != nil {
		return false
	}
	var s choiceState
	return json.Unmarshal(b, &s) == nil && s.OpenDashboard == "always"
}

func rememberAlways(env Env) error {
	b, _ := json.Marshal(choiceState{OpenDashboard: "always"})
	if err := os.MkdirAll(env.StateDir, 0o700); err != nil {
		return fmt.Errorf("choice: %w", err)
	}
	if err := os.WriteFile(filepath.Join(env.StateDir, choiceFile), b, 0o600); err != nil {
		return fmt.Errorf("choice: %w", err)
	}
	return nil
}

// openDashboard starts the web dashboard and opens its link.
func openDashboard(env Env) (string, error) {
	start := env.StartDashboard
	if start == nil {
		start = func() (string, error) { return startDashboard(env) }
	}
	link, err := start()
	if err != nil {
		return "", err
	}
	if err := env.Open(link); err != nil {
		return link, fmt.Errorf("choice: open browser: %w", err)
	}
	return link, nil
}

// startDashboard runs tock web on its own, past this process, until it has
// gone dashIdle unused, and returns its link.
func startDashboard(env Env) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("choice: %w", err)
	}
	if err := os.MkdirAll(env.StateDir, 0o700); err != nil {
		return "", fmt.Errorf("choice: %w", err)
	}
	log := filepath.Join(env.StateDir, "web.log")
	out, err := os.OpenFile(log, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("choice: %w", err)
	}
	defer out.Close()
	if err := detach(self, []string{"web", "--no-open", "--idle", dashIdle}, out); err != nil {
		return "", fmt.Errorf("choice: start dashboard: %w", err)
	}
	return waitLink(env, log)
}

// waitLink reads the link tock web writes first to log.
func waitLink(env Env, log string) (string, error) {
	for range dashTries {
		if b, err := os.ReadFile(log); err == nil {
			// A whole first line: tock web writes it in one go.
			if first, _, whole := strings.Cut(string(b), "\n"); whole {
				if link, ok := strings.CutPrefix(first, "tock web: "); ok && strings.HasPrefix(link, "http://127.0.0.1:") {
					return link, nil
				}
			}
		}
		env.Sleep(dashPoll)
	}
	return "", errors.New("choice: the dashboard did not start (see " + log + ")")
}
