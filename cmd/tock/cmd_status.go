package main

import "fmt"

// cmdStatus prints one line for a status bar.
func cmdStatus(env Env, args []string) error {
	if len(args) > 0 {
		return usagef("status takes no arguments")
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	line, err := a.Status()
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, line)
	return nil
}
