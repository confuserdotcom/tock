package main

import (
	"fmt"
	"os"

	"tock/internal/store"
)

// cmdImport reads another tracker's history into tock.
func cmdImport(env Env, args []string) error {
	fs := newFlags("import")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return usagef("import: want activitywatch <export.json>")
	}
	if fs.Arg(0) != "activitywatch" {
		return usagef("import: unknown source %q, want activitywatch", fs.Arg(0))
	}
	data, err := os.ReadFile(fs.Arg(1))
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	// History can come before trackd ever ran: make tock.db if it is not there.
	st, err := store.Open(env.dbPath())
	if err != nil {
		return err
	}
	st.Close()
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	r, err := a.ImportActivityWatch(data)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "imported %s over %s from ActivityWatch\n", plural(r.Views, "view"), plural(r.Days, "day"))
	return nil
}
