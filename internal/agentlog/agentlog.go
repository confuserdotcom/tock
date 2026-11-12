// Package agentlog reads when coding agents worked, from Claude Code's
// session logs (~/.claude/projects/<dir>/<session>.jsonl and their
// subagents). It reads each entry's type, time, working directory and the
// session's title only: never prompts, replies or tool output. It only
// reads, and nothing it reads leaves the machine.
//
// An Index keeps what it read, by file, so the next update reads only the
// lines appended since; Save and Load keep it between runs.
package agentlog

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// Index is what was read from each log under a root.
type Index struct {
	Version int
	Files   map[string]*FileState
}

// FileState is one log as far as it was read: its size and modification
// time then, the bytes read (whole lines only), and its sessions.
type FileState struct {
	Size, ModNS, Offset int64
	Sessions            map[string]*Session
}

// indexVersion changes when what an Index holds changes; an older one is
// read again from scratch.
const indexVersion = 1

// Read returns the sessions with activity in [from, to) without keeping an
// index. A missing root is no sessions.
func Read(root string, from, to int64) ([]Session, error) {
	ix := &Index{}
	if _, err := ix.Update(root); err != nil {
		return nil, err
	}
	return ix.Sessions(from, to), nil
}

// Load reads an index saved by Save; a missing, unreadable or older one is
// an empty index.
func Load(path string) *Index {
	ix := &Index{}
	f, err := os.Open(path)
	if err != nil {
		return ix
	}
	defer f.Close()
	if gob.NewDecoder(f).Decode(ix) != nil || ix.Version != indexVersion {
		return &Index{}
	}
	return ix
}

// Save writes the index to path, through a temporary file so a reader
// never sees half of it.
func (ix *Index) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("agentlog: save: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agents-*")
	if err != nil {
		return fmt.Errorf("agentlog: save: %w", err)
	}
	ix.Version = indexVersion
	if err := gob.NewEncoder(tmp).Encode(ix); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("agentlog: save: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("agentlog: save: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("agentlog: save: %w", err)
	}
	return nil
}

// Update reads what changed under root since the last update: the new
// lines of a log that grew, a new or rewritten log whole, and forgets logs
// that are gone. It reports whether anything changed.
func (ix *Index) Update(root string) (changed bool, err error) {
	if ix.Files == nil || ix.Version != indexVersion {
		ix.Files, ix.Version = map[string]*FileState{}, indexVersion
	}
	seen := map[string]bool{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			switch {
			case path == root && errors.Is(err, fs.ErrNotExist):
				return fs.SkipAll
			case path == root:
				return err
			case d != nil && d.IsDir():
				return fs.SkipDir // unreadable: skip it, keep the rest
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		seen[path] = true
		st := ix.Files[path]
		size, mod := info.Size(), info.ModTime().UnixNano()
		if st != nil && st.Size == size && st.ModNS == mod {
			return nil
		}
		if st == nil || size < st.Offset {
			st = &FileState{Sessions: map[string]*Session{}}
		}
		if readFrom(path, parentSession(path), st) {
			st.Size, st.ModNS = size, mod
			ix.Files[path] = st
			changed = true
		}
		return nil
	})
	if err != nil {
		return changed, fmt.Errorf("agentlog: read %s: %w", root, err)
	}
	for path := range ix.Files {
		if !seen[path] {
			delete(ix.Files, path)
			changed = true
		}
	}
	return changed, nil
}

// Sessions returns the sessions with activity in [from, to), by first
// activity, a session's logs (its subagents', a resumed one's) joined.
func (ix *Index) Sessions(from, to int64) []Session {
	byID := map[string]*Session{}
	paths := make([]string, 0, len(ix.Files))
	for p := range ix.Files {
		paths = append(paths, p)
	}
	slices.Sort(paths) // a session's own log before its subagents'
	for _, p := range paths {
		for id, part := range ix.Files[p].Sessions {
			s := byID[id]
			if s == nil {
				s = &Session{ID: id}
				byID[id] = s
			}
			if s.Dir == "" {
				s.Dir = part.Dir
			}
			for _, t := range part.Titles {
				if !slices.Contains(s.Titles, t) {
					s.Titles = append(s.Titles, t)
				}
			}
			s.Events = appendIn(s.Events, part.Events, from, to)
			s.Prompts = appendIn(s.Prompts, part.Prompts, from, to)
		}
	}
	var out []Session
	for _, s := range byID {
		if len(s.Events) == 0 {
			continue
		}
		slices.Sort(s.Events)
		s.Events = slices.Compact(s.Events)
		slices.Sort(s.Prompts)
		s.Prompts = slices.Compact(s.Prompts)
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b Session) int {
		if a.Events[0] != b.Events[0] {
			return int(min(max(a.Events[0]-b.Events[0], -1), 1))
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

func appendIn(dst, src []int64, from, to int64) []int64 {
	for _, t := range src {
		if t >= from && t < to {
			dst = append(dst, t)
		}
	}
	return dst
}

// skip are entry types that carry no activity, often large.
var skip = [][]byte{[]byte(`{"type":"api-request`), []byte(`{"type":"file-history`), []byte(`{"type":"queue-operation`)}

// parentSession is the session a subagent log belongs to:
// <session>/subagents/<agent>.jsonl names it by its folder. "" otherwise.
func parentSession(path string) string {
	dir := filepath.Dir(path)
	if filepath.Base(dir) != "subagents" {
		return ""
	}
	return filepath.Base(filepath.Dir(dir))
}

// readFrom reads path's whole lines from st.Offset on into st. A line still
// being written is left for the next read. It reports false when the file
// could not be read.
func readFrom(path, session string, st *FileState) bool {
	f, err := os.Open(path)
	if err != nil {
		return false // gone or unreadable: nothing to count
	}
	defer f.Close()
	if _, err := f.Seek(st.Offset, io.SeekStart); err != nil {
		return false
	}
	r := bufio.NewReaderSize(f, 1<<16)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return true // EOF: a partial line waits for its newline
		}
		st.Offset += int64(len(line))
		if !skipLine(line) {
			take(line, session, st.Sessions)
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

func take(line []byte, session string, byID map[string]*Session) {
	var e entry
	if json.Unmarshal(line, &e) != nil || e.SessionID == "" {
		return
	}
	if session != "" {
		e.SessionID = session
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
	if s.Dir == "" && !e.Sidechain {
		s.Dir = e.Cwd
	}
	s.Events = append(s.Events, at)
	if e.Type == "user" && !e.IsMeta && e.Origin != nil && e.Origin.Kind == "human" {
		s.Prompts = append(s.Prompts, at)
	}
}
