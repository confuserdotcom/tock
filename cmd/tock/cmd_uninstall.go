package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tock/internal/service"
)

// Seams for tests: where tock runs from, and unloading the background jobs.
var (
	selfPath    = os.Executable
	unloadAgent = service.Uninstall
	unloadUnit  = service.UninstallUnit
)

// cmdUninstall stops trackd and the menu bar and removes what make install
// put down. --purge also deletes tock.db, the rules and the config.
func cmdUninstall(env Env, args []string) error {
	fs := newFlags("uninstall")
	purge := fs.Bool("purge", false, "also delete your data, rules and config")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("uninstall: unexpected argument %q", fs.Arg(0))
	}
	if *purge {
		fmt.Fprintf(env.Stdout, "This deletes all tracked time, rules and settings in\n  %s\n  %s\n  %s\nType yes to go on: ",
			env.DataDir, env.StateDir, env.ConfigDir)
		line, _ := bufio.NewReader(env.Stdin).ReadString('\n')
		if strings.TrimSpace(line) != "yes" {
			return errors.New("uninstall: nothing removed")
		}
	}
	self, err := selfPath()
	if err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	bin := filepath.Dir(self)
	var remove []string
	if env.Linux {
		if err := unloadUnit("trackd"); err != nil {
			return fmt.Errorf("uninstall: %w", err)
		}
		remove = append(remove, filepath.Join(bin, "trackd"))
	} else {
		for _, l := range []string{"com.tock.trackd", "com.tock.bar"} {
			if err := unloadAgent(l); err != nil {
				return fmt.Errorf("uninstall: %w", err)
			}
		}
		remove = append(remove, filepath.Join(env.DataDir, "trackd.app"), filepath.Join(env.DataDir, "tock.app"))
	}
	if filepath.Base(self) == "tock" { // never a test binary or go run
		remove = append(remove, self)
	}
	if *purge {
		remove = append(remove, env.DataDir, env.StateDir, env.ConfigDir)
	}
	for _, p := range remove {
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("uninstall: %w", err)
		}
		fmt.Fprintf(env.Stdout, "removed %s\n", p)
	}
	if *purge {
		fmt.Fprintln(env.Stdout, "tock is uninstalled and your data deleted.")
	} else {
		fmt.Fprintf(env.Stdout, "tock is uninstalled. Your data is kept in %s and %s;\ndelete those folders to remove it too.\n", env.DataDir, env.ConfigDir)
	}
	if !env.Linux {
		fmt.Fprintln(env.Stdout, "The signing certificate stays in your login keychain (Keychain Access: tock-local-codesign).")
	}
	return nil
}
