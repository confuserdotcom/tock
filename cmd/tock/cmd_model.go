package main

import (
	"fmt"
	"strings"

	"tock/internal/classify"
	"tock/internal/report"
)

const modelUsage = `model: usage: tock model | tock model why <app> "<title>" [<label>]`

// cmdModel shows how well the local model labels, or why it would label a
// window the way it does.
func cmdModel(env Env, args []string) error {
	if len(args) > 0 && args[0] != "why" || len(args) == 2 || len(args) > 4 {
		return usagef(modelUsage)
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	if len(args) >= 3 {
		label := ""
		if len(args) == 4 {
			label = args[3]
		}
		all, byTitle, without, because, err := a.ModelExplain(args[1], args[2], label)
		if err != nil {
			return err
		}
		if all == nil && byTitle == nil {
			fmt.Fprintln(env.Stdout, "the model knows nothing about this window")
			return nil
		}
		fmt.Fprintf(env.Stdout, "everything     %s\ntitle          %s\n", scored(all), scored(byTitle))
		if label != "" {
			fmt.Fprintf(env.Stdout, "title, not %s  %s\n", label, scored(without))
		}
		if len(because) > 0 {
			var ws []string
			for i, w := range because {
				if i == 5 {
					break
				}
				ws = append(ws, fmt.Sprintf("%s %+.1f", w.Category, w.P))
			}
			fmt.Fprintf(env.Stdout, "words          %s\n", strings.Join(ws, " · "))
		}
		return nil
	}
	st, err := a.ModelStats()
	if err != nil {
		return err
	}
	ag, err := a.ModelAgreement()
	if err != nil {
		return err
	}
	defer printAgreement(env, ag)
	if st.Held == 0 {
		fmt.Fprintln(env.Stdout, "Model  nothing labelled to learn from yet")
		return nil
	}
	fmt.Fprintf(env.Stdout, "Model  %d examples · %d held out\n", st.Examples, st.Held)
	fmt.Fprintf(env.Stdout, "  right       %3d%%  (the app alone: %d%%)\n", pct(st.Right, st.Held), pct(st.AppRight, st.Held))
	fmt.Fprintf(env.Stdout, "  by title    %3d%%  (the app alone: %d%%) on the %d labels that read the title\n",
		pct(st.TitledRight, st.Titled), pct(st.TitledAppRight, st.Titled), st.Titled)
	fmt.Fprintf(env.Stdout, "  sure        %3d%%  of them, right %d%% of the time: these it labels\n", pct(st.Sure, st.Held), pct(st.SureRight, st.Sure))
	fmt.Fprintln(env.Stdout, "\nBy category")
	for _, r := range st.PerCategory {
		fmt.Fprintf(env.Stdout, "  %-18s %4d  %3d%%\n", r.Category, r.Held, pct(r.Right, r.Held))
	}
	if len(st.Confusions) > 0 {
		fmt.Fprintln(env.Stdout, "\nMistaken for")
		for i, c := range st.Confusions {
			if i == 5 {
				break
			}
			fmt.Fprintf(env.Stdout, "  %-18s → %-18s %d\n", c.Label, c.Guess, c.N)
		}
	}
	return nil
}

func pct(n, of int) int {
	if of == 0 {
		return 0
	}
	return (n*100 + of/2) / of
}

func scored(xs []classify.Scored) string {
	var b strings.Builder
	for i, x := range xs {
		if i == 3 || x.P < 0.01 {
			break
		}
		if i > 0 {
			b.WriteString(" · ")
		}
		fmt.Fprintf(&b, "%s %d%%", x.Category, int(x.P*100+0.5))
	}
	if b.Len() == 0 {
		return "-"
	}
	return b.String()
}

// printAgreement is the whole labeller against your hand labels: the one
// number that says how often tock files your time where you would.
func printAgreement(env Env, ag classify.Agreement) {
	if ag.Spans == 0 {
		return
	}
	lo, hi := classify.Wilson(ag.Agreed, ag.Spans)
	spans := "spans"
	if ag.Spans == 1 {
		spans = "span"
	}
	fmt.Fprintf(env.Stdout, "\nYour labels  %d %s · %s, held out from what labels them\n", ag.Spans, spans, report.Dur(ag.MS))
	fmt.Fprintf(env.Stdout, "  agrees      %3d%% of the time  (%d%% of spans, 95%% range %d-%d%%)\n",
		pct64(ag.AgreedMS, ag.MS), pct(ag.Agreed, ag.Spans), int(lo*100+0.5), int(hi*100+0.5))
	fmt.Fprintf(env.Stdout, "  blank       %3d%% of the time\n", pct64(ag.UnlabelledMS, ag.MS))
	fmt.Fprintln(env.Stdout, "  You label by hand mostly what tock missed, so this is a floor.")
	for _, r := range ag.PerCategory {
		prec := "-"
		if r.GuessedMS >= 10*60_000 { // less is noise
			prec = fmt.Sprintf("%d%%", pct64(r.AgreedMS, r.GuessedMS))
		}
		fmt.Fprintf(env.Stdout, "  %-18s %7s  found %3d%%  right when it says so %s\n", r.Category, report.Dur(r.MS), pct64(r.AgreedMS, r.MS), prec)
	}
	for i, c := range ag.Confusions {
		if i == 5 {
			break
		}
		if i == 0 {
			fmt.Fprintln(env.Stdout, "  filed instead under")
		}
		fmt.Fprintf(env.Stdout, "    %-18s → %-18s %s\n", c.Label, c.Guess, report.Dur(c.MS))
	}
}

func pct64(n, of int64) int {
	if of == 0 {
		return 0
	}
	return int((n*100 + of/2) / of)
}
