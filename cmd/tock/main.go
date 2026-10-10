// Command tock reads the spans trackd records, labels them with
// ~/.config/tock/rules.toml and prints where the time went.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/charmbracelet/x/term"

	"tock/internal/claude"
	"tock/internal/suggest"
)

const usage = `usage:
  tock                                   open the TUI (today, week, inbox, focus, rules)
  tock status                            one line for a status bar
  tock init                              write starter ~/.config/tock/rules.toml
  tock today [--date YYYY-MM-DD] [--keys]
  tock inbox [--days N] [--all]          unlabelled activity, numbered
  tock tag [--once] <n> <Category>[/<Project>]
  tock tag --span <id> <Category>[/<Project>]
  tock ignore <n> | tock ignore --undo <key>
  tock rules check
  tock session [start [MIN] | stop]      focus session: show, start (default 25), stop
  tock recap [YYYY-MM-DD]                the daily recap
  tock suggest [--dry-run | --force]     Claude label suggestions for the inbox (needs ANTHROPIC_API_KEY)
  tock doctor [--wait] [--open]          check trackd, permissions, rules and config
  tock export [--from D] [--to D] [--format csv|json]
                                         labelled spans, default the last 7 days as csv
Names match on a unique prefix: "tock tag 2 stu/atl". "/Project" sets only the project.
`

// Env is everything a command touches outside the process, so tests can
// substitute it.
type Env struct {
	Stdin                        io.Reader
	Stdout, Stderr               io.Writer
	Now                          func() time.Time
	DataDir, StateDir, ConfigDir string
	// Interactive is true when stdin and stdout are terminals: `tock` with
	// no arguments then opens the TUI instead of printing usage.
	Interactive bool
	// APIKey is ANTHROPIC_API_KEY. "" turns suggestions off; it is never
	// written anywhere.
	APIKey string
	// NewClient builds the Claude client; tests substitute a fake.
	NewClient func(key string) suggest.Client
	// Open opens a URL with macOS open(1); Sleep waits. Tests substitute
	// both.
	Open  func(url string) error
	Sleep func(time.Duration)
}

func (e Env) dbPath() string      { return filepath.Join(e.DataDir, "tock.db") }
func (e Env) rulesPath() string   { return filepath.Join(e.ConfigDir, "rules.toml") }
func (e Env) learnedPath() string { return filepath.Join(e.ConfigDir, "learned.toml") }
func (e Env) listingPath() string { return filepath.Join(e.StateDir, "listing.json") }

func envDir(name string, def ...string) string {
	if d := os.Getenv(name); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(append([]string{home}, def...)...)
}

func defaultEnv() Env {
	return Env{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		Now:         time.Now,
		Interactive: term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd()),
		DataDir:     envDir("TOCK_DATA_DIR", ".local", "share", "tock"),
		StateDir:    envDir("TOCK_STATE_DIR", ".local", "state", "tock"),
		ConfigDir:   envDir("TOCK_CONFIG_DIR", ".config", "tock"),
		APIKey:      os.Getenv("ANTHROPIC_API_KEY"),
		NewClient:   func(key string) suggest.Client { return claude.New(key) },
		Open:        func(url string) error { return exec.Command("open", url).Run() },
		Sleep:       time.Sleep,
	}
}

// usageError is a bad command line: exit 2.
type usageError string

func (u usageError) Error() string { return string(u) }

func usagef(format string, a ...any) error { return usageError(fmt.Sprintf(format, a...)) }

// newFlags returns a flag set whose parse errors become usageErrors.
func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return usagef("%s: %v", fs.Name(), err)
	}
	return nil
}

var commands = map[string]func(Env, []string) error{
	"init":    cmdInit,
	"rules":   cmdRules,
	"today":   cmdToday,
	"inbox":   cmdInbox,
	"ignore":  cmdIgnore,
	"tag":     cmdTag,
	"status":  cmdStatus,
	"session": cmdSession,
	"recap":   cmdRecap,
	"suggest": cmdSuggest,
	"export":  cmdExport,
	"doctor":  cmdDoctor,
}

// run executes one command and returns the exit code: 0 ok, 1 error, 2 usage.
func run(args []string, env Env) int {
	var err error
	switch {
	case len(args) == 0 && env.Interactive:
		err = cmdTUI(env)
	case len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help":
		fmt.Fprint(env.Stdout, usage)
		return 0
	default:
		cmd, ok := commands[args[0]]
		if !ok {
			fmt.Fprintf(env.Stderr, "tock: unknown command %q\n%s", args[0], usage)
			return 2
		}
		err = cmd(env, args[1:])
	}
	var ue usageError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ue):
		fmt.Fprintf(env.Stderr, "tock: %v\n%s", err, usage)
		return 2
	}
	fmt.Fprintf(env.Stderr, "tock: %v\n", err)
	return 1
}

func main() {
	os.Exit(run(os.Args[1:], defaultEnv()))
}
