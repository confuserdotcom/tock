package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"tock/internal/claude"
	"tock/internal/report"
)

// LLM is the Claude client for summaries and questions; claude.Client is
// one.
type LLM interface {
	Summarize(ctx context.Context, model string, payload []byte) (json.RawMessage, error)
	Ask(ctx context.Context, model string, payload []byte) (json.RawMessage, error)
}

func (e Env) llm() LLM {
	if e.NewLLM != nil {
		return e.NewLLM(e.APIKey)
	}
	return claude.New(e.APIKey)
}

// parseDay reads --date, default today.
func parseDay(env Env, date, cmd string) (time.Time, error) {
	now := env.Now()
	if date == "" {
		return now, nil
	}
	d, err := time.ParseInLocation("2006-01-02", date, now.Location())
	if err != nil {
		return d, usagef("%s: --date wants YYYY-MM-DD", cmd)
	}
	return d, nil
}

// cmdBlocks lists the day's work blocks, with their summaries when saved.
func cmdBlocks(env Env, args []string) error {
	fs := newFlags("blocks")
	date := fs.String("date", "", "day, YYYY-MM-DD (default today)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("blocks takes only --date")
	}
	day, err := parseDay(env, *date, "blocks")
	if err != nil {
		return err
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	d, err := a.Day(day)
	if err != nil {
		return err
	}
	printBlocks(env, d)
	return nil
}

func printBlocks(env Env, d report.Day) {
	loc := env.Now().Location()
	fmt.Fprintf(env.Stdout, "Work blocks  %s\n", d.Date.Format("Mon 2 Jan"))
	if d.Summary != "" {
		fmt.Fprintln(env.Stdout)
		for _, l := range wrap(report.Safe(d.Summary), 76) {
			fmt.Fprintln(env.Stdout, l)
		}
	}
	fmt.Fprintln(env.Stdout)
	if len(d.Blocks) == 0 {
		fmt.Fprintln(env.Stdout, "no block of 15 min or more yet")
		return
	}
	for _, b := range d.Blocks {
		fmt.Fprintln(env.Stdout, report.BlockLine(b, loc))
		if b.Summary != "" {
			fmt.Fprintf(env.Stdout, "               %s\n", report.Safe(b.Summary))
		}
	}
}

// cmdSummary asks Claude what the day was about and saves the answer, or
// prints the saved one.
func cmdSummary(env Env, args []string) error {
	fs := newFlags("summary")
	date := fs.String("date", "", "day, YYYY-MM-DD (default today)")
	dry := fs.Bool("dry-run", false, "print the exact payload and send nothing")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("summary: usage: tock summary [--date D] [--dry-run]")
	}
	day, err := parseDay(env, *date, "summary")
	if err != nil {
		return err
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	b, err := a.SummaryBatch(day)
	if err != nil {
		return err
	}
	if len(b.Starts) == 0 {
		fmt.Fprintln(env.Stdout, "nothing to summarize: no work block of 15 min or more")
		return nil
	}
	if *dry {
		fmt.Fprintf(env.Stdout, "%s\n", b.Payload)
		return nil
	}
	if !a.Config().LLM.Enabled {
		fmt.Fprintln(env.Stdout, "Claude is off: [llm] enabled = false in config.toml")
		return nil
	}
	if env.APIKey == "" {
		return errors.New("ANTHROPIC_API_KEY is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), claude.Timeout)
	defer cancel()
	model := a.Config().LLM.Model
	raw, err := env.llm().Summarize(ctx, model, b.Payload)
	if err != nil {
		return fmt.Errorf("summary failed: %v", err)
	}
	if _, err := a.SaveSummary(day, b, model, raw); err != nil {
		return err
	}
	d, err := a.Day(day)
	if err != nil {
		return err
	}
	printBlocks(env, d)
	return nil
}

// wrap breaks s into lines of at most w runes.
func wrap(s string, w int) []string {
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line != "" && len([]rune(line))+1+len([]rune(word)) > w {
			out = append(out, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}
