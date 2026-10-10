package main

import (
	"bufio"
	"fmt"
	"strings"

	"tock/internal/classify"
	"tock/internal/store"
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
	if fs.NArg() != wantArgs || (*once && *spanID != 0) {
		return usagef("tag: usage: tock tag [--once] <n> <Category>[/<Project>] | tock tag --span <id> <Category>[/<Project>]")
	}
	w, err := openWorld(env)
	if err != nil {
		return err
	}
	defer w.close()
	want, err := parseWant(w.rs, fs.Arg(wantArgs-1))
	if err != nil {
		return err
	}
	if *spanID != 0 {
		return tagSpans(env, w, []int64{*spanID}, want, fmt.Sprintf("span %d", *spanID))
	}

	key, err := resolveListed(env, fs.Arg(0))
	if err != nil {
		return err
	}
	hist, _, ovs, err := w.history()
	if err != nil {
		return err
	}
	var ids []int64
	var ms int64
	for _, v := range hist {
		if classify.Key(v) == key {
			ids = append(ids, v.ID)
			ms += v.DurMS()
		}
	}
	if len(ids) == 0 {
		return fmt.Errorf("no spans with this key any more: run tock inbox again")
	}
	what := fmt.Sprintf("%d spans (%s)", len(ids), fmtDur(ms))
	if *once {
		return tagSpans(env, w, ids, want, what)
	}

	cands, err := w.rs.Propose(key, want, hist, ovs)
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
			return tagSpans(env, w, ids, want, what)
		}
		fmt.Fprintln(env.Stdout, "cancelled")
		return nil
	}
	for i, c := range safe {
		fmt.Fprintf(env.Stdout, "rule: %s  ->  %s\n", c.Rule.Describe(), labelText(want.Category, want.Project))
		fmt.Fprintf(env.Stdout, "matches %d spans (%s): %d this key, %d other unlabelled (%s), %d relabelled\n",
			c.Matches, fmtDur(c.MatchMS), c.ThisKey, c.New, fmtDur(c.NewMS), c.Relabeled)
		more := i+1 < len(safe)
		if more {
			fmt.Fprint(env.Stdout, "[y] accept  [s] more specific  [n] cancel: ")
		} else {
			fmt.Fprint(env.Stdout, "[y] accept  [n] cancel: ")
		}
		switch a := answer(in); {
		case a == 'y':
			comment := fmt.Sprintf("learned %s from %q, %d spans", env.Now().Format("2006-01-02"), key, c.ThisKey)
			if err := classify.AppendLearned(env.learnedPath(), c.Rule, comment); err != nil {
				return err
			}
			fmt.Fprintf(env.Stdout, "learned: %s now covers %d spans (%s)\n",
				labelText(want.Category, want.Project), c.Matches, fmtDur(c.MatchMS))
			return nil
		case a == 's' && more:
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
	catPart, projPart, hasProj := strings.Cut(arg, "/")
	var w classify.Want
	var err error
	if strings.TrimSpace(catPart) == "" && (!hasProj || strings.TrimSpace(projPart) == "") {
		return w, usagef("tag: give a category, Category/Project or /Project")
	}
	if strings.TrimSpace(catPart) != "" {
		if w.Category, err = rs.ResolveCategory(catPart); err != nil {
			return w, err
		}
	}
	if hasProj && strings.TrimSpace(projPart) != "" {
		if w.Project, err = rs.ResolveProject(projPart); err != nil {
			return w, err
		}
	}
	return w, nil
}

func tagSpans(env Env, w *world, ids []int64, want classify.Want, what string) error {
	at := env.Now().UnixMilli()
	ovs := make([]store.Override, len(ids))
	for i, id := range ids {
		ovs[i] = store.Override{SpanID: id, Category: want.Category, Project: want.Project, At: at}
	}
	if err := w.st.SetOverrides(ovs); err != nil {
		if len(ids) == 1 {
			return fmt.Errorf("no span with id %d", ids[0])
		}
		return err
	}
	fmt.Fprintf(env.Stdout, "tagged %s → %s\n", what, labelText(want.Category, want.Project))
	return nil
}
