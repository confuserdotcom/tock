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

	"tock/internal/app"
	"tock/internal/claude"
	"tock/internal/gitlog"
	"tock/internal/shell"
	"tock/internal/slack"
	"tock/internal/suggest"
)

const usage = `usage: tock <command> [flags]

Every day:
  tock                                   open the TUI (today, week, inbox, focus, rules, plan)
  tock today [--date YYYY-MM-DD] [--keys]
  tock inbox [--days N] [--all | --auto]  unlabelled activity, numbered; --auto: what tock labelled itself
  tock tag [--once] <n> [<Category>[/<Project>]]   (no label: accept the → suggestion)
  tock tag --span <id> <Category>[/<Project>]
  tock ignore <n> | tock ignore --undo <key>
  tock away [--date D] [<HH:MM> <Category>[/<Project>] | --skip <HH:MM>]
                                         label time away (idle or asleep) or leave it idle
  tock edit [add | label | delete] <HH:MM-HH:MM> [<Category>[/<Project>] [note]]
                                         fix the timeline: add missing time, label part of it, delete some
  tock edit [--date D] | tock edit remove <id>   list the day's edits | drop one
  tock undo [--list]                     reverse the last tag, rule, ignore, away label or edit
  tock web [--port N] [--no-open] [--idle D]
                                         the dashboard in your browser, served on 127.0.0.1
  tock status                            one line for a status bar

Focus:
  tock session [start [MIN] [--paper] [--goal TEXT] | stop]
                                         focus session: show, start (default 25; --paper: idle is paper work; --goal: what it is for), stop
  tock session snooze [MIN|off]          snooze distraction notices of the running session (default [focus] snooze), or resume them
  tock session list | weights            this week's finished sessions with quality and rating | the quality weights
  tock session rate [#ID|last] N [note...]  rate a finished session 1 to 5, with a one-line note
  tock session note [#ID] [text...]      set or (no text) clear a session's note
  tock session refile [#ID|last] PROJECT  relabel a finished session's active time to a project (tock undo reverts)
  tock break [MIN]                       take a break now (default 5 min)
  tock goal [set <label> <time>[/week] [--max] | rm <label> [week] [--max]]
                                         goals per day or week, on a category or module
  tock recap [YYYY-MM-DD|start|midday]   the daily recap, or a routine notice now

Rules and names:
  tock rules check | tock rules stats [--days N]
                                         validate the rules | time each rule decided (default 30 days)
  tock rules dupes | tock rules merge [--project] <from> <into>
                                         look-alike names | fold one name into another everywhere
  tock category [add <name> <kind> [--afk D] |
                set <name> [--kind K] [--afk D] [--interrupt on|off] |
                rename <name> <new> | rm <name>]
                                         list or change your categories (rules.toml)
  tock project [add <name> [--category C] [--alias a,b] [--rate N] | set <name> ... |
               rename <name> <new> | rm <name> | scan <dir> --category C]
                                         projects; filed under a category they are modules

Looking back:
  tock top [--days N | --week | --month] [--by category|app|site|project] [--limit N]
                                         where the time went: top categories, apps and sites
  tock trends [--month YYYY-MM] [--hours]  a month against the one before; --hours: focus by weekday and hour
  tock blocks [--date D]                 the day's work blocks: stretches of work on one thing
  tock review [--week YYYY-MM-DD] [--plan TEXT]
                                         weekly recap, then next week's focus
  tock report [--week YYYY-MM-DD] [-o FILE] [--open]
                                         the week as one HTML page to keep or mail
  tock export [--from D] [--to D] [--format csv|json] [--by project]
                                         labelled spans, default the last 7 days as csv
  tock model [why <app> "<title>" [<label>]]
                                         how well the local model labels; why it labels a window so

Setup and data:
  tock init                              write starter ~/.config/tock/rules.toml
  tock doctor [--wait] [--open] [--deep] [--json]
                                         check trackd, permissions, rules, config and snapshots;
                                         --deep: a full check of the database
  tock bug-report [-o FILE] [--include-rules]
                                         a file to attach to an issue, without titles or URLs
  tock update [trust <fingerprint>]      Linux: the key trusted for the Mac's builds; trust one
  tock version                           the version of this build
  tock choice [<feature> on|off]         turn features on and off (screen text, wake screen, Slack…),
                                         open the dashboard; piped: each feature and whether it is on
  tock backup [--out FILE]               archive tock.db and the rule, config and shell files
                                         (default ~/.local/share/tock/backups/)
  tock restore [<n|path> [--yes]]        list snapshots of the database, or put one back
                                         (stops and starts trackd)
  tock prune [--days N] [--dry-run]      delete window titles (and URL paths) older than N days
                                         (default [privacy] keep_titles); labels stay
  tock import activitywatch <export.json>
                                         ActivityWatch history (Export all buckets, or
                                         curl localhost:5600/api/0/export); again replaces it
  tock setup [--check] [--no-doctor]    after brew install: make the signing certificate, install
                                         and start trackd and the menu bar app (safe to run again)
  tock uninstall [--purge]              stop trackd and the menu bar, remove the apps and tock;
                                         --purge also deletes your data, rules and config
  tock shell-init zsh                    shell hook for terminal projects; in ~/.zshrc:
                                         eval "$(tock shell-init zsh)"

Extras (optional; some need a setting, Claude or Slack):
  tock plan                              the TUI on the plan tab ([plan] todo.txt); piped: the open tasks
  tock suggest [--dry-run | --force]     Claude label suggestions for the inbox (claude CLI or ANTHROPIC_API_KEY)
  tock summary [--date D] [--dry-run]    what you worked on: Claude writes it, else the work blocks
  tock ask [--dry-run] "<question>"      ask about your time in plain words; answered on this machine
  tock recommend [accept | dismiss]      the pending suggestion: show it, take it, or not now
  tock ocr test [--now] | grant          read the front window's text once (in 3 s) | ask for Screen Recording
  tock invoice --project P [--from D] [--to D] [--client NAME] [-o FILE]
                                         preview an invoice from the project's rate (DRAFT PDF; default last month)
  tock invoice issue|void|list|show|check|verify|recover …
                                         issue and keep invoices (tock invoice help)
  tock sync [--days N]                   write this machine's last N days (default 2) to the
                                         [sync] folder now and read the other machines'
  tock slack [login | logout | status]   Slack status and do-not-disturb during focus sessions

Names match on a unique prefix: "tock tag 2 stu/atl". "/Project" sets only the project.
tock <command> --help shows one command.
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
	// ClaudeCLI is the claude binary used when APIKey is "": Claude on the
	// user's subscription. "" means none was found.
	ClaudeCLI string
	// NewCLI builds the CLI backend; tests substitute a fake.
	NewCLI func(path string) AI
	// StartDashboard starts the web dashboard and returns its link; nil
	// runs tock web on its own.
	StartDashboard func() (string, error)
	// Open opens a URL with macOS open(1); Sleep waits. Tests substitute
	// both.
	Open  func(url string) error
	Sleep func(time.Duration)
	// Git runs git for the commit counts; nil runs the git on PATH.
	Git gitlog.Runner
	// DirExists reports whether a directory is there; nil checks the disk.
	DirExists func(string) bool
	// NewLLM builds the client for summaries and questions; nil: Claude.
	NewLLM func(key string) LLM
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
	// Hypridle reports whether hypridle runs trackd idle (Linux); nil: no.
	Hypridle func() bool
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
		ClaudeCLI:   claude.FindCLI(),
		SlackToken:  os.Getenv("TOCK_SLACK_TOKEN"),
		ListShortcuts: func() ([]string, error) {
			out, err := exec.Command("/usr/bin/shortcuts", "list").Output()
			if err != nil {
				return nil, err
			}
			return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
		},
		NewClient: func(key string) suggest.Client { return claude.New(key) },
		Open: func(url string) error {
			if runtime.GOOS == "linux" {
				return exec.Command("xdg-open", url).Run()
			}
			return exec.Command("open", url).Run()
		},
		Sleep: time.Sleep,
		Linux: runtime.GOOS == "linux",
		Hypridle: func() bool {
			home, _ := os.UserHomeDir()
			return app.HypridleHooked(home)
		},
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
		if errors.Is(err, flag.ErrHelp) {
			return errHelp
		}
		return usagef("%s: %v", fs.Name(), err)
	}
	return nil
}

// errHelp is -h or --help reaching a command's flags (tock session start
// --help): run prints the command's help and exits 0.
var errHelp = errors.New("help requested")

var commands = map[string]func(Env, []string) error{
	"init":       cmdInit,
	"rules":      cmdRules,
	"category":   cmdCategory,
	"goal":       cmdGoal,
	"project":    cmdProject,
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
	"bug-report": cmdBugReport,
	"backup":     cmdBackup,
	"restore":    cmdRestore,
	"sync":       cmdSync,
	"shell-init": cmdShellInit,
	"away":       cmdAway,
	"trends":     cmdTrends,
	"recommend":  cmdRecommend,
	"ocr":        cmdOCR,
	"top":        cmdTop,
	"model":      cmdModel,
	"review":     cmdReview,
	"report":     cmdReport,
	"invoice":    cmdInvoice,
	"prune":      cmdPrune,
	"web":        cmdWeb,
	"slack":      cmdSlack,
	"edit":       cmdEdit,
	"blocks":     cmdBlocks,
	"summary":    cmdSummary,
	"ask":        cmdAsk,
	"undo":       cmdUndo,
	"plan":       cmdPlan,
	"choice":     cmdChoice,
	"break":      cmdBreak,
	"import":     cmdImport,
	"uninstall":  cmdUninstall,
	"setup":      cmdSetup,
	"update":     cmdUpdate,
}

// run executes one command and returns the exit code: 0 ok, 1 error, 2
// usage, or the code of an exitStatus (tock invoice check: 3, verify: 4).
func run(args []string, env Env) int {
	var err error
	switch {
	case len(args) == 0 && env.Interactive:
		err = cmdTUI(env)
	case len(args) == 1 && (args[0] == "version" || args[0] == "--version"):
		fmt.Fprintf(env.Stdout, "tock %s\n", version)
		return 0
	case len(args) == 2 && args[0] == "help":
		if _, ok := commands[args[1]]; !ok {
			fmt.Fprintf(env.Stderr, "tock: unknown command %q\n%s", args[1], unknownHelp(args[1]))
			return 2
		}
		fmt.Fprint(env.Stdout, commandHelp(args[1]))
		return 0
	case len(args) == 0 || args[0] == "help" || isHelpFlag(args[0]):
		fmt.Fprint(env.Stdout, usage)
		return 0
	default:
		cmd, ok := commands[args[0]]
		if !ok {
			fmt.Fprintf(env.Stderr, "tock: unknown command %q\n%s", args[0], unknownHelp(args[0]))
			return 2
		}
		if wantsHelp(args[0], args[1:]) {
			fmt.Fprint(env.Stdout, commandHelp(args[0]))
			return 0
		}
		err = cmd(env, args[1:])
	}
	var ue usageError
	var es exitStatus
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errHelp) && len(args) > 0:
		fmt.Fprint(env.Stdout, commandHelp(args[0]))
		return 0
	case errors.As(err, &es):
		if es.msg != "" {
			fmt.Fprintf(env.Stderr, "tock: %s\n", es.msg)
		}
		return es.code
	case errors.As(err, &ue):
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		fmt.Fprintf(env.Stderr, "tock: %v\n%s", err, helpFor(name, err))
		return 2
	}
	fmt.Fprintf(env.Stderr, "tock: %v\n", err)
	return 1
}

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], defaultEnv()))
}
