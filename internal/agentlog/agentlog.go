// Package agentlog reads when coding agents worked, from Claude Code's
// session logs (~/.claude/projects/<dir>/<session>.jsonl and their
// subagents). It reads each entry's type, time, working directory and the
// session's title only: never prompts, replies or tool output. It only
// reads, and nothing it reads leaves the machine.
package agentlog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Session is one agent session: its working directory, its titles in the
// order they were set, and the times of its activity, ascending.
type Session struct {
	ID      string
	Dir     string
	Titles  []string
	Events  []int64 // every prompt, reply and tool result, subagents included
	Prompts []int64 // the prompts a person typed
}

// DefaultDir is where Claude Code keeps its session logs under home.
func DefaultDir(home string) string { return filepath.Join(home, ".claude", "projects") }

type entry struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
	SessionID string `json:"sessionId"`
	AITitle   string `json:"aiTitle"`
	IsMeta    bool   `json:"isMeta"`
	Sidechain bool   `json:"isSidechain"`
	Origin    *struct {
		Kind string `json:"kind"`
	} `json:"origin"`
}

// Read returns the sessions with activity in [from, to), by first activity.
// It skips logs not written since from. A missing root is no sessions.
func Read(root string, from, to int64) ([]Session, error) {
	byID := map[string]*Session{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().UnixMilli() < from {
			return nil
		}
		return readFile(path, from, to, byID)
	})
	if err != nil {
		return nil, fmt.Errorf("agentlog: read %s: %w", root, err)
	}
	var out []Session
	for _, s := range byID {
		if len(s.Events) == 0 {
			continue
		}
		slices.Sort(s.Events)
		s.Events = slices.Compact(s.Events)
		slices.Sort(s.Prompts)
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b Session) int {
		if a.Events[0] != b.Events[0] {
			return int(min(max(a.Events[0]-b.Events[0], -1), 1))
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

// skip are entry types that carry no activity, often large.
var skip = [][]byte{[]byte(`{"type":"api-request`), []byte(`{"type":"file-history`), []byte(`{"type":"queue-operation`)}

func readFile(path string, from, to int64, byID map[string]*Session) error {
	f, err := os.Open(path)
	if err != nil {
		return nil // gone or unreadable: nothing to count
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<16)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && !skipLine(line) {
			take(line, from, to, byID)
		}
		if err != nil {
			return nil
		}
	}
}

func skipLine(line []byte) bool {
	for _, p := range skip {
		if bytes.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

func take(line []byte, from, to int64, byID map[string]*Session) {
	var e entry
	if json.Unmarshal(line, &e) != nil || e.SessionID == "" {
		return
	}
	s := byID[e.SessionID]
	if s == nil {
		s = &Session{ID: e.SessionID}
		byID[e.SessionID] = s
	}
	if e.Type == "ai-title" {
		if t := strings.TrimSpace(e.AITitle); t != "" && !slices.Contains(s.Titles, t) {
			s.Titles = append(s.Titles, t)
		}
		return
	}
	if e.Type != "user" && e.Type != "assistant" {
		return
	}
	t, err := time.Parse(time.RFC3339Nano, e.Timestamp)
	if err != nil {
		return
	}
	at := t.UnixMilli()
	if at < from || at >= to {
		return
	}
	if s.Dir == "" && !e.Sidechain {
		s.Dir = e.Cwd
	}
	s.Events = append(s.Events, at)
	if e.Type == "user" && !e.IsMeta && e.Origin != nil && e.Origin.Kind == "human" {
		s.Prompts = append(s.Prompts, at)
	}
}
