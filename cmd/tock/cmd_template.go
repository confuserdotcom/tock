package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/report"
	"tock/internal/rulesfile"
)

// setupTemplates plans the named templates, prints the plan, asks (unless
// yes) and applies it. Without past their rules start now. Before tock has
// a database it applies without undo or a look at the past.
func setupTemplates(env Env, names []string, past, yes bool) error {
	in := bufio.NewReader(env.Stdin)
	cfg, err := config.Load(env.configPath())
	if err != nil {
		cfg = config.Default() // a broken config.toml says so elsewhere; groups read as on
	}
	files := rulesfile.Files{Rules: env.rulesPath(), Learned: env.learnedPath()}
	a, err := openApp(env)
	noDB := errors.Is(err, app.ErrNoDB)
	if err != nil && !noDB {
		return err
	}
	if a != nil {
		defer a.Close()
	}
	if !past && !yes && env.Interactive && !noDB {
		fmt.Fprint(env.Stdout, "Label future activity only, or also past activity? [F/p] ")
		line, _ := in.ReadString('\n')
		past = strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "p")
	}
	if past && noDB {
		past = false // no history yet
	}
	now := env.Now().UnixMilli()
	var p app.TemplatePlan
	if noDB {
		p, err = app.PlanTemplatesNoDB(files, cfg, names, now)
	} else {
		p, err = a.PlanTemplates(names, past)
	}
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	printTemplatePlan(env.Stdout, p)
	if p.Empty() {
		fmt.Fprintln(env.Stdout, "nothing to change")
		return nil
	}
	if !yes {
		if !env.Interactive {
			return usagef("setup: confirm with --yes")
		}
		fmt.Fprint(env.Stdout, "Apply? [y/N] ")
		line, _ := in.ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			fmt.Fprintln(env.Stdout, "nothing changed")
			return nil
		}
	}
	if noDB {
		p, err = app.ApplyTemplatesNoDB(files, cfg, names, now)
	} else {
		p, err = a.ApplyTemplates(names, past)
	}
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	if p.Empty() {
		fmt.Fprintln(env.Stdout, "nothing to change")
		return nil
	}
	cats := "categories"
	if len(p.AddCategories) == 1 {
		cats = "category"
	}
	msg := fmt.Sprintf("added %d %s and %s to rules.toml", len(p.AddCategories), cats, plural(len(p.AddRules), "rule"))
	if !noDB {
		msg += "; tock undo takes them back"
	}
	fmt.Fprintln(env.Stdout, msg)
	return nil
}

// printTemplatePlan says what a template apply would change, and what of
// yours it leaves alone.
func printTemplatePlan(w io.Writer, p app.TemplatePlan) {
	list := func(head string, xs []string) {
		if len(xs) > 0 {
			fmt.Fprintf(w, "  %s: %s\n", head, strings.Join(xs, ", "))
		}
	}
	fmt.Fprintf(w, "Templates: %s\n", strings.Join(p.Names, ", "))
	list("categories to add", p.AddCategories)
	list("categories already there", p.HaveCategories)
	for _, c := range p.KeptCategories {
		fmt.Fprintf(w, "  kept your %s (its kind stays)\n", c)
	}
	if len(p.AddRules) > 0 {
		fmt.Fprintf(w, "  rules to add (%d):\n", len(p.AddRules))
		for _, r := range p.AddRules {
			fmt.Fprintf(w, "    %s → %s\n", report.Safe(r.Describe()), report.LabelText(r.Category, r.Project))
		}
	}
	dated := 0
	for _, r := range p.HaveRules {
		if p.Past && r.SinceMS != 0 {
			fmt.Fprintf(w, "  already present from %s: %s (remove its since line to apply it to the past)\n",
				classify.SinceShort(r.SinceMS), report.Safe(r.Describe()))
			dated++
		}
	}
	if n := len(p.HaveRules) - dated; n > 0 {
		fmt.Fprintf(w, "  %s already there\n", plural(n, "rule"))
	}
	switch {
	case len(p.AddRules) == 0:
	case p.Past:
		fmt.Fprintf(w, "  effect on the last %d days: labels %s that is unlabelled now, and changes %s the local model labelled\n",
			p.Days, report.Dur(p.PastNewMS), report.Dur(p.PastChangedMS))
	default:
		fmt.Fprintln(w, "  new rules label activity from now on; past days keep their labels")
	}
	for _, g := range p.Groups {
		if !g.On {
			fmt.Fprintf(w, "  %s works best with %s on; it is off in config.toml (kept). Turn it on: tock choice %s on\n", g.Template, g.Group, g.Group)
		}
	}
}

// chooseTemplates asks a fresh install what the computer is for. Not on a
// terminal it answers "own", which is closest to tock without templates.
func chooseTemplates(env Env, prompt bool) []string {
	if !prompt || !env.Interactive {
		return []string{"own"}
	}
	all := classify.AllTemplates()
	fmt.Fprintln(env.Stdout, "What do you use this computer for? (one or more, e.g. 1,2)")
	for i, t := range all {
		fmt.Fprintf(env.Stdout, "  %d) %s\n", i+1, t.Title)
	}
	fmt.Fprintf(env.Stdout, "[%d] ", len(all))
	line, _ := bufio.NewReader(env.Stdin).ReadString('\n')
	var out []string
	for _, f := range strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\r' }) {
		var n int
		if _, err := fmt.Sscan(f, &n); err == nil && n >= 1 && n <= len(all) {
			out = append(out, all[n-1].Name)
		}
	}
	if len(out) == 0 {
		return []string{all[len(all)-1].Name}
	}
	return out
}
