package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"tock/internal/collect"
	"tock/internal/span"
)

func dumpLines(t *testing.T, ppid int, ins ...span.Input) []string {
	t.Helper()
	var buf bytes.Buffer
	if rc := runDump(collect.NewFake(ins), &buf, ppid); rc != 0 {
		t.Fatalf("runDump rc = %d", rc)
	}
	return strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
}

func TestDumpBannerFirstLine(t *testing.T) {
	lines := dumpLines(t, 1)
	if !regexp.MustCompile(`^# trackd dump version=\S+ pid=\d+ ppid=\d+$`).MatchString(lines[0]) {
		t.Fatalf("banner = %q", lines[0])
	}
}

func TestDumpTerminalNotice(t *testing.T) {
	lines := dumpLines(t, 4242)
	if len(lines) < 2 || !strings.HasPrefix(lines[1], "# permissions shown are those of the launching terminal") {
		t.Fatalf("terminal launch: lines = %q", lines)
	}
	ev := span.Input{Event: &span.Event{TS: 1700000000000, Kind: span.EventLock}}
	lines = dumpLines(t, 1, ev)
	if len(lines) != 2 || strings.HasPrefix(lines[1], "#") {
		t.Fatalf("launchd launch: want banner then event, got %q", lines)
	}
}

func TestFormatPrivateSampleHasNoTitleOrURL(t *testing.T) {
	title, url := "Secret", "secret.example/x"
	line := formatInput(span.Input{Sample: &span.Sample{
		TS: 1700000000000, BundleID: "com.google.Chrome", App: "Chrome",
		Title: &title, URL: &url, Private: true,
	}})
	if !strings.Contains(line, "title=- url=- private=1") {
		t.Fatalf("private line = %q", line)
	}
	if strings.Contains(line, title) || strings.Contains(line, url) {
		t.Fatalf("private line leaks data: %q", line)
	}
}

func TestFormatQuotesControlChars(t *testing.T) {
	title := "\x1b[31mred"
	line := formatInput(span.Input{Sample: &span.Sample{
		TS: 1700000000000, BundleID: "a", App: "A\x1b", Title: &title,
	}})
	if strings.ContainsRune(line, 0x1b) {
		t.Fatalf("raw ESC in output: %q", line)
	}
	if !strings.Contains(line, `title="\x1b[31mred"`) {
		t.Fatalf("title not %%q-escaped: %q", line)
	}
}

func TestFormatEventLine(t *testing.T) {
	line := formatInput(span.Input{Event: &span.Event{TS: 1700000000000, Kind: span.EventLock}})
	if !regexp.MustCompile(`^\S+ event lock$`).MatchString(line) {
		t.Fatalf("event line = %q", line)
	}
}
