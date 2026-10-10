package main

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"tock/internal/classify"
)

// cmdInit writes the starter rules.toml; it never overwrites.
func cmdInit(env Env, args []string) error {
	if len(args) > 0 {
		return usagef("init takes no arguments")
	}
	path := env.rulesPath()
	if err := os.MkdirAll(env.ConfigDir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s already exists; not overwriting", path)
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(classify.Starter); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "wrote %s\n", path)
	return nil
}

// cmdRules implements "tock rules check".
func cmdRules(env Env, args []string) error {
	if len(args) != 1 || args[0] != "check" {
		return usagef("usage: tock rules check")
	}
	rs, err := classify.Load(env.rulesPath(), env.learnedPath())
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "rules.toml: %d categories, %d projects, %d rules\nlearned.toml: %d rules\n",
		len(rs.Categories), len(rs.Projects), len(rs.Rules), len(rs.Learned))

	st, err := openStore(env)
	if errors.Is(err, errNoDB) {
		return nil
	}
	if err != nil {
		return err
	}
	defer st.Close()
	ovs, err := st.Overrides()
	if err != nil {
		return err
	}
	stale := map[string]int{}
	for _, o := range ovs {
		if o.Category != "" && !rs.HasCategory(o.Category) {
			stale[fmt.Sprintf("category %q", o.Category)]++
		}
		if o.Project != "" && !rs.HasProject(o.Project) {
			stale[fmt.Sprintf("project %q", o.Project)]++
		}
	}
	names := make([]string, 0, len(stale))
	for n := range stale {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(env.Stdout, "warning: %d manual tags name undeclared %s\n", stale[n], n)
	}
	return nil
}
