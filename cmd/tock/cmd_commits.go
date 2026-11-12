package main

import (
	"fmt"
	"strings"
	"time"

	"tock/internal/report"
)

const commitsUsage = "commits: usage: tock commits [--days N | --week] [--project P]"

// cmdCommits lists the commits of the project repos with the project time
// that went into each one.
func cmdCommits(env Env, args []string) error {
	fs := newFlags("commits")
	days := fs.Int("days", 1, "the last N days, today included")
	week := fs.Bool("week", false, "this week, from Monday")
	project := fs.String("project", "", "one project only")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 || *days < 1 || *days > 366 {
		return usagef(commitsUsage)
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
	r, err := a.CommitLog(from, to)
	if err != nil {
		return err
	}
	for _, l := range commitLines(r, *project, to.Sub(from) > 24*time.Hour, now.Location()) {
		fmt.Fprintln(env.Stdout, l)
	}
	return nil
}

// commitLines renders r: one line per commit, then the time since each
// project's last commit. With days set each line starts with its date.
func commitLines(r report.CommitReport, project string, days bool, loc *time.Location) []string {
	var rows []report.CommitRow
	projW, hashW := 0, 0
	for _, row := range r.Rows {
		if project != "" && row.Project != project {
			continue
		}
		rows = append(rows, row)
		projW = max(projW, len([]rune(report.Safe(row.Project))))
		hashW = max(hashW, len(row.Hash))
	}
	if len(rows) == 0 {
		return []string{"no commits in this range"}
	}
	agents := false
	for _, row := range rows {
		agents = agents || row.AgentMS > 0
	}
	stamp := func(ms int64) string {
		at := time.UnixMilli(ms).In(loc)
		if days {
			return at.Format("Mon 2 15:04")
		}
		return at.Format("15:04")
	}
	// The header names the time columns: you, and agents when they worked.
	head := fmt.Sprintf("%*s  %5s", len([]rune(stamp(rows[0].AtMS)))+2+projW+2+hashW, "", "you")
	if agents {
		head += fmt.Sprintf("  %5s", "agent")
	}
	out := []string{head}
	var total, agentTotal int64
	for _, row := range rows {
		subj := []rune(report.Safe(row.Subject))
		if len(subj) > 60 {
			subj = append(subj[:59], '…')
		}
		line := fmt.Sprintf("%s  %-*s  %-*s  %5s", stamp(row.AtMS), projW, report.Safe(row.Project), hashW, row.Hash, report.Dur(row.WorkMS))
		if agents {
			line += fmt.Sprintf("  %5s", report.Dur(row.AgentMS))
		}
		out = append(out, line+"  "+string(subj))
		total += row.WorkMS
		agentTotal += row.AgentMS
	}
	sum := fmt.Sprintf("%d commits · %s of your time behind them", len(rows), report.Dur(total))
	if agents {
		sum += fmt.Sprintf(", %s of agents'", report.Dur(agentTotal))
	}
	out = append(out, sum)
	var open []string
	for _, o := range r.Open {
		if project == "" || o.Name == project {
			open = append(open, report.Safe(o.Name)+" "+report.Dur(o.MS))
		}
	}
	if len(open) > 0 {
		out = append(out, "since last commit  "+strings.Join(open, " · "))
	}
	return out
}
