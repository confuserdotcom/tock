package main

import (
	"fmt"
	"time"

	"tock/internal/report"
)

const agentsUsage = "agents: usage: tock agents [--days N | --week]"

// cmdAgents is the agent ledger: what coding agents worked on and how much
// of it you watched.
func cmdAgents(env Env, args []string) error {
	fs := newFlags("agents")
	days := fs.Int("days", 1, "the last N days, today included")
	week := fs.Bool("week", false, "this week, from Monday")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 || *days < 1 || *days > 366 {
		return usagef(agentsUsage)
	}
	now := env.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	to := today.AddDate(0, 0, 1)
	from := to.AddDate(0, 0, -*days)
	if *week {
		from = today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	ag, err := a.Agents(from, to)
	if err != nil {
		return err
	}
	for _, l := range agentLines(ag) {
		fmt.Fprintln(env.Stdout, l)
	}
	return nil
}

func agentLines(ag report.Agents) []string {
	if len(ag.Rows) == 0 {
		return []string{"no agent sessions in this range (tock reads Claude Code's logs in ~/.claude/projects)"}
	}
	w := len("project")
	for _, r := range ag.Rows {
		w = max(w, len([]rune(report.Safe(r.Project))))
	}
	out := []string{fmt.Sprintf("%-*s  %6s  %6s  %7s  %8s", w, "project", "agent", "you", "prompts", "sessions")}
	for _, r := range ag.Rows {
		out = append(out, fmt.Sprintf("%-*s  %6s  %6s  %7d  %8d", w, report.Safe(r.Project),
			report.Dur(r.AgentMS), report.Dur(r.YouMS), r.Prompts, r.Sessions))
	}
	line := fmt.Sprintf("agents worked %s", report.Dur(ag.AgentMS))
	if ag.Peak > 1 {
		line += fmt.Sprintf(" (%s of clock time, up to %d at once)", report.Dur(ag.WallMS), ag.Peak)
	}
	line += fmt.Sprintf(" · you %s in their windows", report.Dur(ag.YouMS))
	if ag.YouMS > 0 && ag.AgentMS > 0 {
		line += fmt.Sprintf(" · %.1f× leverage", float64(ag.AgentMS)/float64(ag.YouMS))
	}
	return append(out, line)
}
