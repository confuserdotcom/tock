package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"tock/internal/config"
)

// bugReportLogLines is how much of each log goes in.
const bugReportLogLines = 500

// bugReportRunDays is how far back trackd's runs go in.
const bugReportRunDays = 14

// privateLine matches log lines that could carry a window title or a URL;
// they are left out whole.
var privateLine = regexp.MustCompile(`(?i)https?://|\btitle\b|\burl\b|\bwindow\b`)

// cmdBugReport writes a .tar.gz a user can look through and attach to an
// issue: doctor --json, config.toml with every text value removed, the end
// of the logs without lines that could hold a title or URL, and trackd's
// runs. It sends nothing.
func cmdBugReport(env Env, args []string) error {
	fs := newFlags("bug-report")
	out := fs.String("o", "", "write the archive here (default ~/.local/state/tock/bug-report-<time>.tar.gz)")
	withRules := fs.Bool("include-rules", false, "add rules.toml (your app, site and project names)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("bug-report takes only flags")
	}
	path := *out
	if path == "" {
		path = filepath.Join(env.StateDir, "bug-report-"+env.Now().Format("20060102-150405")+".tar.gz")
	}
	scrub := scrubber(env)

	type member struct {
		name string
		body []byte
	}
	var members []member
	add := func(name string, b []byte) { members = append(members, member{name, []byte(scrub(string(b)))}) }

	var doc bytes.Buffer
	denv := env
	denv.Stdout, denv.Stderr, denv.Open = &doc, io.Discard, nil
	_ = cmdDoctor(denv, []string{"--json"}) // problems are what the report is for
	add("doctor.json", doc.Bytes())

	if b, err := os.ReadFile(env.configPath()); err == nil {
		add("config.toml", []byte(redactTOML(string(b))))
	}
	for _, l := range []string{"trackd.log", "web.log", "tockbar.log"} {
		if b, err := logTail(filepath.Join(env.StateDir, l)); err == nil {
			add(l, b)
		}
	}
	if b, err := bugReportRuns(env); err == nil {
		add("trackd_runs.json", b)
	}
	if *withRules {
		if b, err := os.ReadFile(env.rulesPath()); err == nil {
			add("rules.toml", b)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("bug-report: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("bug-report: %w", err)
	}
	zw := gzip.NewWriter(f)
	tw := tar.NewWriter(zw)
	for _, m := range members {
		h := &tar.Header{Name: m.name, Mode: 0o600, Size: int64(len(m.body)), ModTime: env.Now()}
		if err := tw.WriteHeader(h); err != nil {
			f.Close()
			return fmt.Errorf("bug-report: %w", err)
		}
		if _, err := tw.Write(m.body); err != nil {
			f.Close()
			return fmt.Errorf("bug-report: %w", err)
		}
	}
	err = tw.Close()
	if zerr := zw.Close(); err == nil {
		err = zerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("bug-report: %w", err)
	}

	fmt.Fprintf(env.Stdout, "wrote %s with:\n", path)
	for _, m := range members {
		fmt.Fprintf(env.Stdout, "  %-18s %s\n", m.name, sizeKB(len(m.body)))
	}
	fmt.Fprintf(env.Stdout, "Nothing was sent. Look through it first (tar xzf %s), then attach it to an issue.\n", filepath.Base(path))
	if !*withRules {
		fmt.Fprintln(env.Stdout, "Your rules are not in it; --include-rules adds rules.toml if the problem is about labels.")
	}
	return nil
}

func sizeKB(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f KB", float64(n)/1024)
}

// tomlString matches a TOML string literal.
var tomlString = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"|'([^']*)'`)

// scrubber replaces, in every member, each text value of config.toml with
// <removed> (doctor quotes them in its errors), the sync folder with
// <sync> and the home folder with ~: paths say who you are and where you
// sync.
func scrubber(env Env) func(string) string {
	var olds []string
	repl := map[string]string{}
	if c, err := config.Load(env.configPath()); err == nil && c.Sync.Dir != "" {
		olds, repl[c.Sync.Dir] = append(olds, c.Sync.Dir), "<sync>"
	}
	if src, err := os.ReadFile(env.configPath()); err == nil {
		for _, m := range tomlString.FindAllStringSubmatch(string(src), -1) {
			v := m[1] + m[2]
			if len(v) < 3 { // "", "on", short words: nothing private, and too common to replace
				continue
			}
			if _, dup := repl[v]; !dup {
				olds, repl[v] = append(olds, v), "<removed>"
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil && len(home) > 1 {
		olds, repl[home] = append(olds, home), "~"
	}
	// Longest first, so a value that contains another is replaced whole.
	sort.SliceStable(olds, func(a, b int) bool { return len(olds[a]) > len(olds[b]) })
	var pairs []string
	for _, o := range olds {
		pairs = append(pairs, o, repl[o])
	}
	return strings.NewReplacer(pairs...).Replace
}

// redactTOML keeps config.toml's tables, keys, numbers and booleans and
// replaces every string and array with "<removed>": names, addresses, the
// sync folder, hidden apps and sites. Comments go.
func redactTOML(src string) string {
	var b strings.Builder
	depth := 0 // inside a multi-line array
	for _, line := range strings.Split(src, "\n") {
		t := strings.TrimSpace(line)
		if depth > 0 {
			depth += strings.Count(t, "[") - strings.Count(t, "]")
			continue
		}
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, "[") {
			b.WriteString(t + "\n")
			continue
		}
		key, val, ok := strings.Cut(t, "=")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch {
		case strings.HasPrefix(val, "["):
			depth = strings.Count(val, "[") - strings.Count(val, "]")
			val = `"<removed>"`
		case strings.HasPrefix(val, `"`) || strings.HasPrefix(val, "'") || strings.HasPrefix(val, "{"):
			val = `"<removed>"`
		default:
			if i := strings.Index(val, "#"); i >= 0 {
				val = strings.TrimSpace(val[:i])
			}
		}
		b.WriteString(key + " = " + val + "\n")
	}
	return b.String()
}

// logTail is the last bugReportLogLines lines of a log, without the lines
// privateLine matches.
func logTail(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if privateLine.MatchString(sc.Text()) {
			continue
		}
		lines = append(lines, sc.Text())
		if len(lines) > bugReportLogLines {
			lines = lines[1:]
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// bugReportRuns is trackd's runs of the last two weeks: when it ran,
// stopped and why, never what it recorded.
func bugReportRuns(env Env) ([]byte, error) {
	st, err := openStore(env)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	now := env.Now()
	runs, err := st.Runs(now.Add(-bugReportRunDays*24*time.Hour).UnixMilli(), now.UnixMilli())
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(runs, "", "  ")
}
