package app

import (
	"context"
	"time"

	"tock/internal/classify"
	"tock/internal/gitlog"
	"tock/internal/report"
)

// gitLiveFor is how long commit times of a range that includes now stay
// cached.
const gitLiveFor = 60 * time.Second

type gitKey struct {
	repo     string
	from, to int64
}

type gitEntry struct {
	times []int64
	at    time.Time
	live  bool // the range includes now
}

// SetGitRunner replaces the git runner (tests) and drops the cache.
func (a *App) SetGitRunner(r gitlog.Runner) {
	a.git, a.gitCache = r, nil
}

// commits counts each project's commits in [from, to). A repo that fails
// counts nothing; tock doctor reports it.
func (a *App) commits(from, to int64) []report.NameN {
	by := map[string]int{}
	for _, p := range a.rs.Projects {
		for _, r := range p.Repos {
			by[p.Name] += len(a.commitTimes(classify.ExpandHome(r, a.home), from, to))
		}
	}
	return report.Commits(by)
}

func (a *App) commitTimes(repo string, from, to int64) []int64 {
	now := a.now()
	k := gitKey{repo, from, to}
	if e, ok := a.gitCache[k]; ok && (!e.live || now.Sub(e.at) < gitLiveFor) {
		return e.times
	}
	run := a.git
	if run == nil {
		run = gitlog.Exec
	}
	times, err := gitlog.Commits(context.Background(), run, repo, from, to)
	if err != nil {
		times = nil
	}
	if a.gitCache == nil {
		a.gitCache = map[gitKey]gitEntry{}
	}
	a.gitCache[k] = gitEntry{times: times, at: now, live: to > now.UnixMilli()}
	return times
}

// RepoCheck is one configured repo and the error reading it, nil when git
// reads it.
type RepoCheck struct {
	Project, Repo string
	Err           error
}

// CheckRepos checks every configured repo, in file order.
func (a *App) CheckRepos() []RepoCheck {
	run := a.git
	if run == nil {
		run = gitlog.Exec
	}
	var out []RepoCheck
	for _, p := range a.rs.Projects {
		for _, r := range p.Repos {
			out = append(out, RepoCheck{p.Name, r, gitlog.Check(context.Background(), run, classify.ExpandHome(r, a.home))})
		}
	}
	return out
}

// CommitCapMS bounds how far back a commit's window reaches when the
// project's previous commit is older.
const CommitCapMS = 8 * 3600_000

// CommitLog lists the commits of every project repo in [from, to) with the
// project time that went into each, and the time since each project's last
// commit. It reads hashes and subjects for display only.
func (a *App) CommitLog(from, to time.Time) (report.CommitReport, error) {
	f, t := from.UnixMilli(), to.UnixMilli()
	now := a.nowMS()
	t = min(t, max(now, f))
	vs, labels, err := a.labelled(f-CommitCapMS, t, now)
	if err != nil {
		return report.CommitReport{}, err
	}
	run := a.git
	if run == nil {
		run = gitlog.Exec
	}
	var cs []report.ProjectCommit
	for _, p := range a.rs.Projects {
		for _, r := range p.Repos {
			log, err := gitlog.Log(context.Background(), run, classify.ExpandHome(r, a.home), f-CommitCapMS, t)
			if err != nil {
				continue // tock doctor reports it
			}
			for _, c := range log {
				cs = append(cs, report.ProjectCommit{Project: p.Name, AtMS: c.AtMS, Hash: c.Hash, Subject: c.Subject})
			}
		}
	}
	return report.CommitTime(cs, vs, labels, f, t, CommitCapMS), nil
}
