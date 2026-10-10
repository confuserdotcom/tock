package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"tock/internal/ask"
	"tock/internal/claude"
)

// cmdAsk answers a question about tracked time. Claude turns the question
// into a query; tock runs it here, so no activity leaves the machine.
func cmdAsk(env Env, args []string) error {
	fs := newFlags("ask")
	dry := fs.Bool("dry-run", false, "print the exact payload and send nothing")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	question := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(question) == "" {
		return usagef(`ask: usage: tock ask [--dry-run] "how much video last week?"`)
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	payload, err := a.AskPayload(question)
	if err != nil {
		return err
	}
	if *dry {
		fmt.Fprintf(env.Stdout, "%s\n", payload)
		return nil
	}
	if !a.Config().LLM.Enabled {
		return errors.New("Claude is off: [llm] enabled = false in config.toml")
	}
	if env.APIKey == "" {
		return errors.New("ANTHROPIC_API_KEY is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), claude.Timeout)
	defer cancel()
	raw, err := env.llm().Ask(ctx, a.Config().LLM.Model, payload)
	if err != nil {
		return fmt.Errorf("ask failed: %v", err)
	}
	q, r, err := a.AskAnswer(raw)
	if err != nil {
		return err
	}
	for _, l := range ask.Lines(q, r) {
		fmt.Fprintln(env.Stdout, l)
	}
	return nil
}
