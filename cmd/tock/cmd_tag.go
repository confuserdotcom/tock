package main

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

func cmdTag(env Env, args []string) error {
	fs := newFlags("tag")
	once := fs.Bool("once", false, "label these spans only, without learning a rule")
	spanID := fs.Int64("span", 0, "label one span by id")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	wantArgs := 2
	if *spanID != 0 {
		wantArgs = 1
	}
	// "tock tag <n>" accepts the inbox suggestion for n.
	suggested := *spanID == 0 && fs.NArg() == 1
	if (fs.NArg() != wantArgs && !suggested) || (*once && *spanID != 0) {
		return usagef("tag: usage: tock tag [--once] <n> [<Category>[/<Project>]] | tock tag --span <id> <Category>[/<Project>]")
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	if *spanID != 0 {
		want, err := parseWant(a.Rules(), fs.Arg(0))
		if err != nil {
			return err
		}
		return tagSpans(env, a, []int64{*spanID}, want, fmt.Sprintf("span %d", *spanID))
	}

	key, err := resolveListed(env, fs.Arg(0))
	if err != nil {
		return err
	}
	var want classify.Want
	if suggested {
		sugg, err := a.Suggestions()
		if err != nil {
			return err
		}
		var ok bool
		if want, ok = sugg[key]; !ok {
			return fmt.Errorf("no suggestion for %s: give a label, tock tag %s <Category>[/<Project>]", report.Safe(key), fs.Arg(0))
		}
	} else if want, err = parseWant(a.Rules(), fs.Arg(1)); err != nil {
		return err
	}
	ids, ms, err := a.KeySpans(key)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("no spans with this key any more: run tock inbox again")
	}
	what := fmt.Sprintf("%d spans (%s)", len(ids), report.Dur(ms))
	if *once {
		return tagSpans(env, a, ids, want, what)
	}

	cands, err := a.Propose(key, want)
	if err != nil {
		return err
	}
	in := bufio.NewReader(env.Stdin)
	var safe []classify.Candidate
	for _, c := range cands {
		if c.Safe {
			safe = append(safe, c)
		}
	}
	if len(safe) == 0 {
		fmt.Fprintln(env.Stdout, "no safe rule: every candidate would relabel spans other rules already label.")
		fmt.Fprintf(env.Stdout, "[o] tag these %s only  [n] cancel: ", what)
		if answer(in) == 'o' {
			return tagSpans(env, a, ids, want, what)
		}
		fmt.Fprintln(env.Stdout, "cancelled")
		return nil
	}
	for i, c := range safe {
		fmt.Fprintf(env.Stdout, "rule: %s  ->  %s\n", c.Rule.Describe(), report.LabelText(want.Category, want.Project))
		fmt.Fprintf(env.Stdout, "matches %d spans (%s): %d this key, %d other unlabelled (%s), %d relabelled%s\n",
			c.Matches, report.Dur(c.MatchMS), c.ThisKey, c.New, report.Dur(c.NewMS), c.Relabeled, report.FixedText(c.Fixed, len(c.Replaces)))
		more := i+1 < len(safe)
		if more {
			fmt.Fprint(env.Stdout, "[y] accept  [s] more specific  [n] cancel: ")
		} else {
			fmt.Fprint(env.Stdout, "[y] accept  [n] cancel: ")
		}
		switch ans := answer(in); {
		case ans == 'y':
			if err := a.Learn(key, c, want); err != nil {
				return err
			}
			fmt.Fprintf(env.Stdout, "learned: %s now covers %d spans (%s)\n",
				report.LabelText(want.Category, want.Project), c.Matches, report.Dur(c.MatchMS))
			return nil
		case ans == 's' && more:
			continue
		default:
			fmt.Fprintln(env.Stdout, "cancelled")
			return nil
		}
	}
	return nil
}

// answer reads one line and returns its first letter, lowercased (0 at EOF).
func answer(in *bufio.Reader) byte {
	line, _ := in.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		return 0
	}
	return line[0]
}

// parseWant resolves "Category", "Category/Project" or "/Project".
func parseWant(rs *classify.Ruleset, arg string) (classify.Want, error) {
	w, err := rs.ParseWant(arg)
	if errors.Is(err, classify.ErrNoLabel) {
		return w, usagef("tag: %v", err)
	}
	return w, err
}

func tagSpans(env Env, a *app.App, ids []int64, want classify.Want, what string) error {
	if err := a.Tag(ids, want); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "tagged %s → %s\n", what, report.LabelText(want.Category, want.Project))
	return nil
}
