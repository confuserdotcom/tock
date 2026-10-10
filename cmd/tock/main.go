// Command tock reads the spans trackd records, labels them with
// ~/.config/tock/rules.toml and prints where the time went.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const usage = `usage:
  tock init                              write starter ~/.config/tock/rules.toml
  tock today [--date YYYY-MM-DD] [--keys]
  tock inbox [--days N] [--all]          unlabelled activity, numbered
  tock tag [--once] <n> <Category>[/<Project>]
  tock tag --span <id> <Category>[/<Project>]
  tock ignore <n> | tock ignore --undo <key>
  tock rules check
Names match on a unique prefix: "tock tag 2 stu/atl". "/Project" sets only the project.
`

// Env is everything a command touches outside the process, so tests can
// substitute it.
type Env struct {
	Stdin                        io.Reader
	Stdout, Stderr               io.Writer
	Now                          func() time.Time
	DataDir, StateDir, ConfigDir string
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
		Now:       time.Now,
		DataDir:   envDir("TOCK_DATA_DIR", ".local", "share", "tock"),
		StateDir:  envDir("TOCK_STATE_DIR", ".local", "state", "tock"),
		ConfigDir: envDir("TOCK_CONFIG_DIR", ".config", "tock"),
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
	"init":  cmdInit,
	"rules": cmdRules,
}

// run executes one command and returns the exit code: 0 ok, 1 error, 2 usage.
func run(args []string, env Env) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(env.Stdout, usage)
		return 0
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(env.Stderr, "tock: unknown command %q\n%s", args[0], usage)
		return 2
	}
	err := cmd(env, args[1:])
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
