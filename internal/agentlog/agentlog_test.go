package agentlog

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func ms(s string) int64 {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return t.UnixMilli()
}

const session = `{"type":"mode","mode":"normal","sessionId":"s1"}
{"parentUuid":null,"type":"user","isMeta":true,"timestamp":"2026-11-12T10:00:00.000Z","cwd":"/u/tock","sessionId":"s1","message":{"role":"user","content":"meta"}}
{"parentUuid":"a","type":"user","origin":{"kind":"human"},"timestamp":"2026-11-12T10:00:05.000Z","cwd":"/u/tock","sessionId":"s1","message":{"role":"user","content":"secret prompt text"}}
{"parentUuid":"b","type":"assistant","timestamp":"2026-11-12T10:00:09.000Z","cwd":"/u/tock","sessionId":"s1","message":{"content":[{"type":"text","text":"secret"}]}}
{"type":"api-request-blob","sessionId":"s1","hash":"x","message":{"big":"blob"}}
{"parentUuid":"c","type":"user","timestamp":"2026-11-12T10:00:20.000Z","cwd":"/u/tock","sessionId":"s1","message":{"content":[{"type":"tool_result"}]}}
{"type":"ai-title","aiTitle":"Fix the inbox","sessionId":"s1"}
not json
{"type":"ai-title","aiTitle":"Fix the inbox","sessionId":"s1"}
{"type":"ai-title","aiTitle":"Inbox undo","sessionId":"s1"}
{"parentUuid":"d","type":"assistant","timestamp":"2026-11-12T09:00:00.000Z","cwd":"/u/tock","sessionId":"s1","message":{}}
`

// A subagent's log has a session id of its own; it belongs to the session
// whose folder holds it.
const sub = `{"parentUuid":null,"isSidechain":true,"type":"assistant","timestamp":"2026-11-12T10:00:15.000Z","cwd":"/u/tock/web","sessionId":"agent-own-id","message":{}}
`

func TestRead(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "-u-tock")
	if err := os.MkdirAll(filepath.Join(p, "s1", "subagents"), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, s string, mod time.Time) {
		f := filepath.Join(p, name)
		if err := os.WriteFile(f, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(f, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	now := time.UnixMilli(ms("2026-11-12T12:00:00Z"))
	write("s1.jsonl", session, now)
	write("s1/subagents/agent-1.jsonl", sub, now)
	write("old.jsonl", session, now.AddDate(0, 0, -30)) // untouched since from: skipped

	got, err := Read(dir, ms("2026-11-12T09:30:00Z"), ms("2026-11-12T11:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Session{{
		ID: "s1", Dir: "/u/tock", Titles: []string{"Fix the inbox", "Inbox undo"},
		Events:  []int64{ms("2026-11-12T10:00:00Z"), ms("2026-11-12T10:00:05Z"), ms("2026-11-12T10:00:09Z"), ms("2026-11-12T10:00:15Z"), ms("2026-11-12T10:00:20Z")},
		Prompts: []int64{ms("2026-11-12T10:00:05Z")},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
	// An unreadable folder is skipped, not an error.
	locked := filepath.Join(dir, "-locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o700)
	if got, err := Read(dir, ms("2026-11-12T09:30:00Z"), ms("2026-11-12T11:00:00Z")); err != nil || len(got) != 1 {
		t.Fatalf("with a locked folder: %d sessions, %v", len(got), err)
	}
	if s, err := Read(filepath.Join(dir, "missing"), 0, 1); err != nil || s != nil {
		t.Fatalf("missing dir: %v %v", s, err)
	}
}
