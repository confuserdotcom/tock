package app

import (
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"tock/internal/agentlog"
	"tock/internal/classify"
	"tock/internal/report"
)

// AgentGapMS is the longest pause inside an agent's work; a longer one is
// the agent waiting for you.
const AgentGapMS = 3 * 60_000

// Agents is the agent ledger of [from, to): what coding agents worked on,
// by project, and your time in their windows.
func (a *App) Agents(from, to time.Time) (report.Agents, error) {
	f, t := from.UnixMilli(), to.UnixMilli()
	now := a.nowMS()
	t = min(t, max(now, f))
	ss, err := a.agentSessions(f, t)
	if err != nil {
		return report.Agents{}, err
	}
	vs, labels, err := a.labelled(f, t, now)
	if err != nil {
		return report.Agents{}, err
	}
	in := make([]report.AgentSession, len(ss))
	for i, s := range ss {
		in[i] = report.AgentSession{ID: s.ID, Project: a.projectOfDir(s.Dir), Titles: s.Titles, Events: s.Events, Prompts: s.Prompts}
	}
	return report.AgentLedger(in, vs, labels, f, t, AgentGapMS), nil
}

// TempDirs names the sessions that ran in a temporary folder, together.
const TempDirs = "(temp folders)"

var tempRoots = []string{"/tmp", "/private/tmp", "/var/folders", "/private/var/folders"}

// projectOfDir is the project whose repo holds dir, the deepest repo
// winning; else TempDirs for a temporary folder; else dir itself, "~/…"
// under home.
func (a *App) projectOfDir(dir string) string {
	best, bestLen := "", -1
	for _, p := range a.rs.Projects {
		for _, r := range p.Repos {
			r = filepath.Clean(classify.ExpandHome(r, a.home))
			if (dir == r || strings.HasPrefix(dir, r+string(filepath.Separator))) && len(r) > bestLen {
				best, bestLen = p.Name, len(r)
			}
		}
	}
	if best != "" {
		return best
	}
	for _, t := range tempRoots {
		if dir == t || strings.HasPrefix(dir, t+"/") {
			return TempDirs
		}
	}
	if a.home != "" && (dir == a.home || strings.HasPrefix(dir, a.home+string(filepath.Separator))) {
		return "~" + dir[len(a.home):]
	}
	if dir == "" {
		return "(unknown)"
	}
	return dir
}

// agentRefresh is how often agentSessions looks for new log lines.
const agentRefresh = 30 * time.Second

// agentSessions is the agent sessions with activity in [from, to). It
// reads only what was appended since it last looked, and keeps the index
// in Paths.AgentIndex when set.
func (a *App) agentSessions(from, to int64) ([]agentlog.Session, error) {
	if a.agentIx == nil {
		a.agentIx = &agentlog.Index{}
		if a.paths.AgentIndex != "" {
			a.agentIx = agentlog.Load(a.paths.AgentIndex)
		}
	}
	if now := a.now(); a.agentAt.IsZero() || now.Sub(a.agentAt) >= agentRefresh {
		root := a.paths.Agents
		if root == "" {
			root = agentlog.DefaultDir(a.home)
		}
		changed, err := a.agentIx.Update(root)
		if err != nil {
			return nil, err
		}
		a.agentAt = now
		if changed && a.paths.AgentIndex != "" {
			if err := a.agentIx.Save(a.paths.AgentIndex); err != nil {
				return nil, err
			}
		}
	}
	return a.agentIx.Sessions(from, to), nil
}

// attachAgents gives terminal views without a working directory the one of
// the agent session they show (see agentlog.Attach). A log that cannot be
// read leaves them as they are.
func (a *App) attachAgents(vs []classify.SpanView) {
	if !a.cfg.Shell.Agents || a.paths.AgentIndex == "" || len(vs) == 0 {
		return
	}
	lo, hi := int64(math.MaxInt64), int64(math.MinInt64)
	for _, v := range vs {
		if v.Dir == nil && slices.Contains(a.cfg.Shell.Terminals, v.BundleID) {
			lo, hi = min(lo, v.Start), max(hi, v.End)
		}
	}
	if lo > hi {
		return
	}
	ss, err := a.agentSessions(lo-agentlog.AroundAfter, hi+agentlog.AroundBefore)
	if err != nil {
		return
	}
	agentlog.Attach(vs, ss, a.cfg.Shell.Terminals, a.home)
}
