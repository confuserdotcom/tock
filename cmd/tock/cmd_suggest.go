package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"tock/internal/claude"
	"tock/internal/suggest"
)

func cmdSuggest(env Env, args []string) error {
	fs := newFlags("suggest")
	dry := fs.Bool("dry-run", false, "print the exact payload and send nothing")
	force := fs.Bool("force", false, "ignore the one-batch-per-hour limit")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 || (*dry && *force) {
		return usagef("suggest: usage: tock suggest [--dry-run | --force]")
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	if *dry {
		b, err := a.SuggestBatch()
		if err != nil {
			return err
		}
		if len(b.Keys) == 0 {
			fmt.Fprintln(env.Stdout, "nothing to send")
			return nil
		}
		fmt.Fprintf(env.Stdout, "%s\n", b.Payload)
		return nil
	}
	if env.APIKey == "" {
		return errors.New("ANTHROPIC_API_KEY is not set")
	}
	p, err := a.SuggestPrepare(*force)
	if err != nil {
		return err
	}
	switch p.Status {
	case suggest.Off:
		fmt.Fprintln(env.Stdout, "suggestions are off: [llm] enabled = false in config.toml")
		return nil
	case suggest.Nothing:
		fmt.Fprintln(env.Stdout, "nothing to send")
		return nil
	case suggest.Wait:
		at := time.UnixMilli(p.NextMS).In(env.Now().Location()).Format("15:04")
		fmt.Fprintf(env.Stdout, "next batch after %s (tock suggest --force)\n", at)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), claude.Timeout)
	defer cancel()
	raw, err := env.NewClient(env.APIKey).Suggest(ctx, p.Model, p.Batch.Payload)
	if err != nil {
		return fmt.Errorf("suggestions failed: %v", err)
	}
	n, err := a.SuggestSave(p.Batch, p.Model, raw)
	if err != nil {
		return err
	}
	keys := "keys"
	if len(p.Batch.Keys) == 1 {
		keys = "key"
	}
	fmt.Fprintf(env.Stdout, "sent %d %s · %d suggested\n", len(p.Batch.Keys), keys, n)
	return nil
}
