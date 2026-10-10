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
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"

	"tock/internal/claude"
	"tock/internal/gitlog"
	"tock/internal/shell"
	"tock/internal/slack"
	"tock/internal/suggest"
)

const usage = `usage:
  tock                                   open the TUI (today, week, inbox, focus, rules)
  tock status                            one line for a status bar
  tock init                              write starter ~/.config/tock/rules.toml
  tock today [--date YYYY-MM-DD] [--keys]
  tock inbox [--days N] [--all]          unlabelled activity, numbered
  tock tag [--once] <n> [<Category>[/<Project>]]   (no label: accept the → suggestion)
  tock tag --span <id> <Category>[/<Project>]
  tock ignore <n> | tock ignore --undo <key>
  tock away [--date D] [<HH:MM> <Category>[/<Project>] | --skip <HH:MM>]
                                         label time away (idle or asleep) or leave it idle
  tock rules check | tock rules stats [--days N]
                                         validate the rules | time each rule decided (default 30 days)
  tock rules dupes | tock rules merge [--project] <from> <into>
                                         look-alike names | fold one name into another everywhere
  tock session [start [MIN] | stop]      focus session: show, start (default 25), stop
  tock recap [YYYY-MM-DD]                the daily recap
  tock trends [--month YYYY-MM]          a month against the one before
  tock top [--days N | --week | --month] [--by category|app|site|project] [--limit N]
                                         where the time went: top categories, apps and sites
  tock web [--port N] [--no-open]       the dashboard in your browser, on this machine only
  tock review [--week YYYY-MM-DD] [--plan TEXT]
                                         weekly recap, then next week's focus
  tock suggest [--dry-run | --force]     Claude label suggestions for the inbox (needs ANTHROPIC_API_KEY)
  tock doctor [--wait] [--open]          check trackd, permissions, rules and config
  tock export [--from D] [--to D] [--format csv|json] [--by project]
                                         labelled spans, default the last 7 days as csv
  tock backup [--out FILE]               archive tock.db and the rule, config and shell files
                                         (default ~/.local/share/tock/backups/)
  tock sync [--days N]                   write this machine's last N days (default 2) to the
                                         [sync] folder now and read the other machines'
  tock slack [login | logout | status]   Slack status and do-not-disturb during focus sessions
  tock shell-init zsh                    shell hook for terminal projects; in ~/.zshrc:
                                         eval "$(tock shell-init zsh)"
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
	// Git runs git for the commit counts; nil runs the git on PATH.
	Git gitlog.Runner
	// SlackToken is TOCK_SLACK_TOKEN; NewSlack builds the Slack client
	// (nil: the real one).
	SlackToken string
	NewSlack   func(token string) *slack.Client
	// ListShortcuts lists the Shortcuts app's shortcuts; nil skips the
	// check.
	ListShortcuts func() ([]string, error)
	// Linux is true on Linux (Hyprland): doctor then checks the systemd
	// service and skips the macOS permissions.
	Linux bool
}

func (e Env) dbPath() string       { return filepath.Join(e.DataDir, "tock.db") }
func (e Env) rulesPath() string    { return filepath.Join(e.ConfigDir, "rules.toml") }
func (e Env) learnedPath() string  { return filepath.Join(e.ConfigDir, "learned.toml") }
func (e Env) listingPath() string  { return filepath.Join(e.StateDir, "listing.json") }
func (e Env) shellLogPath() string { return filepath.Join(e.StateDir, shell.File) }

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
		SlackToken:  os.Getenv("TOCK_SLACK_TOKEN"),
		ListShortcuts: func() ([]string, error) {
			out, err := exec.Command("/usr/bin/shortcuts", "list").Output()
			if err != nil {
				return nil, err
			}
			return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
		},
		NewClient: func(key string) suggest.Client { return claude.New(key) },
		Open:      func(url string) error { return exec.Command("open", url).Run() },
		Sleep:     time.Sleep,
		Linux:     runtime.GOOS == "linux",
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
	"init":       cmdInit,
	"rules":      cmdRules,
	"today":      cmdToday,
	"inbox":      cmdInbox,
	"ignore":     cmdIgnore,
	"tag":        cmdTag,
	"status":     cmdStatus,
	"session":    cmdSession,
	"recap":      cmdRecap,
	"suggest":    cmdSuggest,
	"export":     cmdExport,
	"doctor":     cmdDoctor,
	"backup":     cmdBackup,
	"sync":       cmdSync,
	"shell-init": cmdShellInit,
	"away":       cmdAway,
	"trends":     cmdTrends,
	"top":        cmdTop,
	"review":     cmdReview,
	"web":        cmdWeb,
	"slack":      cmdSlack,
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
