package app

import (
	"path/filepath"
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
	root := a.paths.Agents
	if root == "" {
		root = agentlog.DefaultDir(a.home)
	}
	ss, err := agentlog.Read(root, f, t)
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
