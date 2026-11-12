package report

import (
	"sort"

	"tock/internal/classify"
	"tock/internal/span"
)

// ProjectCommit is one commit in a repo of Project.
type ProjectCommit struct {
	Project string
	AtMS    int64
	Hash    string
	Subject string
}

// CommitRow is a commit and the project time that went into it.
type CommitRow struct {
	ProjectCommit
	FromMS int64 // where its window starts: the previous commit, or the cap
	WorkMS int64 // your time on the project in [FromMS, AtMS)
	// AgentMS is the clock time coding agents worked on the project in the
	// window (see AgentCommitTime).
	AgentMS int64
}

// CommitReport is the commits of a range, oldest first, and per project the
// time since its last commit (Open), most first.
type CommitReport struct {
	Rows []CommitRow
	Open []NameMS
}

// CommitTime gives each commit in [from, to) the non-idle time labelled with
// its project between the project's previous commit and it, looking back at
// most capMS. Commits before from only bound the window of the next one.
// Open is the project time after each project's last commit, up to to, also
// capped.
func CommitTime(commits []ProjectCommit, views []classify.SpanView, labels []classify.Label, from, to, capMS int64) CommitReport {
	byProj := map[string][]ProjectCommit{}
	for _, c := range commits {
		byProj[c.Project] = append(byProj[c.Project], c)
	}
	var r CommitReport
	for proj, cs := range byProj {
		sort.SliceStable(cs, func(i, j int) bool { return cs[i].AtMS < cs[j].AtMS })
		prev := int64(-1 << 62)
		for _, c := range cs {
			if c.AtMS >= from && c.AtMS < to {
				w := max(prev, c.AtMS-capMS)
				r.Rows = append(r.Rows, CommitRow{ProjectCommit: c, FromMS: w, WorkMS: projectMS(views, labels, proj, w, c.AtMS)})
			}
			prev = c.AtMS
		}
		if ms := projectMS(views, labels, proj, max(prev, to-capMS), to); ms > 0 {
			r.Open = append(r.Open, NameMS{proj, ms})
		}
	}
	sort.SliceStable(r.Rows, func(i, j int) bool {
		if r.Rows[i].AtMS != r.Rows[j].AtMS {
			return r.Rows[i].AtMS < r.Rows[j].AtMS
		}
		return r.Rows[i].Project < r.Rows[j].Project
	})
	sort.Slice(r.Open, func(i, j int) bool {
		if r.Open[i].MS != r.Open[j].MS {
			return r.Open[i].MS > r.Open[j].MS
		}
		return r.Open[i].Name < r.Open[j].Name
	})
	return r
}

// projectMS is the non-idle time labelled proj inside [from, to).
func projectMS(views []classify.SpanView, labels []classify.Label, proj string, from, to int64) int64 {
	var ms int64
	for i, v := range views {
		if v.BundleID == span.IdleBundleID || labels[i].Project != proj {
			continue
		}
		if s, e := max(v.Start, from), min(v.End, to); e > s {
			ms += e - s
		}
	}
	return ms
}

// AgentCommitTime sets each row's AgentMS: the clock time its project's
// agent sessions worked inside the row's window, sessions at once counted
// once. gapMS is as in AgentLedger.
func AgentCommitTime(r *CommitReport, sessions []AgentSession, gapMS int64) {
	for i := range r.Rows {
		row := &r.Rows[i]
		var busy [][2]int64
		for _, s := range sessions {
			if s.Project == row.Project {
				busy = append(busy, busySpans(s.Events, row.FromMS, row.AtMS, gapMS)...)
			}
		}
		row.AgentMS, _ = sweep(busy)
	}
}
