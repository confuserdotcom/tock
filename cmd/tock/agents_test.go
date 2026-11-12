package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentsCmd(t *testing.T) {
	te := newTestEnv(t, repoRules)
	te.seed(t, daySpans()...)
	dir := t.TempDir()
	te.AgentDir = dir
	if code, out := te.run(t, "", "agents"); code != 0 || !strings.Contains(out, "no agent sessions") {
		t.Fatalf("empty: exit %d:\n%s", code, out)
	}
	var b strings.Builder
	for m := 0; m <= 40; m += 2 {
		ts := time.UnixMilli(at(t, fmt.Sprintf("09:%02d", m))).UTC().Format(time.RFC3339Nano)
		extra := ""
		if m == 0 {
			extra = `,"origin":{"kind":"human"}`
		}
		typ := map[bool]string{true: "user", false: "assistant"}[m == 0]
		fmt.Fprintf(&b, `{"type":%q,"timestamp":%q,"cwd":"/r/tock","sessionId":"s"%s}`+"\n", typ, ts, extra)
	}
	os.MkdirAll(filepath.Join(dir, "p"), 0o700)
	if err := os.WriteFile(filepath.Join(dir, "p", "s.jsonl"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out := te.run(t, "", "agents")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, te.errb)
	}
	for _, want := range []string{"project   agent", "tock        40m      0s        1         1", "agents worked 40m · you 0s"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
	if code, _ := te.run(t, "", "agents", "x"); code != 2 {
		t.Fatal("extra arg must be usage")
	}
}
