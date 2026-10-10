package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/span"
)

const (
	unclassified = "Unclassified"
	keysPerRow   = 5
	topProjects  = 3
)

type catRow struct {
	name     string
	ms       int64
	projects map[string]int64
}

func cmdToday(env Env, args []string) error {
	fs := newFlags("today")
	date := fs.String("date", "", "day to show, YYYY-MM-DD (default today)")
	withKeys := fs.Bool("keys", false, "list the top keys of each category, numbered for tock tag")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("today: unexpected argument %q", fs.Arg(0))
	}

	now := env.Now()
	day := now
	if *date != "" {
		d, err := time.ParseInLocation("2006-01-02", *date, now.Location())
		if err != nil {
			return usagef("today: --date wants YYYY-MM-DD")
		}
		day = d
	}
	from := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, now.Location())
	to := from.AddDate(0, 0, 1)

	w, err := openWorld(env)
	if err != nil {
		return err
	}
	defer w.close()
	vs, labels, err := w.labelled(from.UnixMilli(), to.UnixMilli())
	if err != nil {
		return err
	}
	ignored, err := w.st.Ignored()
	if err != nil {
		return err
	}

	var trackedMS, idleMS int64
	rows := map[string]*catRow{}
	for i, v := range vs {
		if v.BundleID == span.IdleBundleID {
			idleMS += v.DurMS()
			continue
		}
		trackedMS += v.DurMS()
		name := labels[i].Category
		if name == "" {
			name = unclassified
		}
		r := rows[name]
		if r == nil {
			r = &catRow{name: name, projects: map[string]int64{}}
			rows[name] = r
		}
		r.ms += v.DurMS()
		if p := labels[i].Project; p != "" {
			r.projects[p] += v.DurMS()
		}
	}

	title := "Today"
	if *date != "" {
		title = from.Format("2006-01-02")
	}
	fmt.Fprintf(env.Stdout, "%s  %s tracked · %s idle\n", title, fmtDur(trackedMS), fmtDur(idleMS))
	if len(rows) == 0 {
		fmt.Fprintln(env.Stdout, "(no activity)")
		return nil
	}

	order := make([]*catRow, 0, len(rows))
	var maxMS int64
	for _, r := range rows {
		order = append(order, r)
		maxMS = max(maxMS, r.ms)
	}
	sort.Slice(order, func(a, b int) bool {
		// Unclassified always last; otherwise by time, then name.
		if (order[a].name == unclassified) != (order[b].name == unclassified) {
			return order[b].name == unclassified
		}
		if order[a].ms != order[b].ms {
			return order[a].ms > order[b].ms
		}
		return order[a].name < order[b].name
	})

	var listed []string
	for _, r := range order {
		extra := projectSummary(r.projects)
		var keep func(i int) bool
		if r.name == unclassified {
			keep = func(i int) bool {
				return labels[i].Category == "" && vs[i].BundleID != span.IdleBundleID && !ignored[classify.Key(vs[i])]
			}
			if n := len(classify.Group(vs, keep)); n > 0 {
				extra = fmt.Sprintf("%d keys → tock inbox", n)
			}
		} else {
			name := r.name
			keep = func(i int) bool { return labels[i].Category == name }
		}
		line := fmt.Sprintf("%s%5s  %s  %s", pad(r.name, 15), fmtDur(r.ms), pad(bar(r.ms, maxMS), barCells), extra)
		fmt.Fprintln(env.Stdout, strings.TrimRight(line, " "))
		if *withKeys {
			items := classify.Group(vs, keep)
			for _, it := range items[:min(len(items), keysPerRow)] {
				listed = append(listed, it.Key)
				fmt.Fprintf(env.Stdout, "    %3d  %5s  %s\n", len(listed), fmtDur(it.TotalMS), safe(it.Key))
			}
		}
	}
	if *withKeys {
		return saveListing(env, listed)
	}
	return nil
}

// projectSummary renders the top projects as "tock 1h40 · ATLAS 34m".
func projectSummary(p map[string]int64) string {
	names := make([]string, 0, len(p))
	for n := range p {
		names = append(names, n)
	}
	sort.Slice(names, func(a, b int) bool {
		if p[names[a]] != p[names[b]] {
			return p[names[a]] > p[names[b]]
		}
		return names[a] < names[b]
	})
	parts := make([]string, 0, topProjects)
	for _, n := range names[:min(len(names), topProjects)] {
		parts = append(parts, n+" "+fmtDur(p[n]))
	}
	return strings.Join(parts, " · ")
}
