// Package gitlog reads commits from local git repositories. Commits reads
// author times only; Log also reads the short hash and subject, for local
// display only (never stored, never sent anywhere). It never fetches.
package gitlog

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Timeout bounds one repo's reads.
const Timeout = 3 * time.Second

// Runner runs git with args in dir and returns its standard output.
type Runner func(ctx context.Context, dir string, args ...string) ([]byte, error)

// Exec runs the git on PATH. Optional locks are off, so a read never takes
// the index lock, and git never prompts.
func Exec(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, firstLine(msg))
		}
		return nil, err
	}
	return out, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Commit is one commit read by Log.
type Commit struct {
	AtMS          int64
	Hash, Subject string
}

// Log is Commits with each commit's short hash and subject.
func Log(ctx context.Context, run Runner, repo string, from, to int64) ([]Commit, error) {
	out, err := logRaw(ctx, run, repo, from, to, "%at%x1f%h%x1f%s")
	if err != nil {
		return nil, err
	}
	var cs []Commit
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, "\x1f", 3)
		s, err := strconv.ParseInt(f[0], 10, 64)
		if err != nil || len(f) != 3 {
			return nil, fmt.Errorf("git log: unexpected line %q", line)
		}
		if ms := s * 1000; ms >= from && ms < to {
			cs = append(cs, Commit{AtMS: ms, Hash: f[1], Subject: f[2]})
		}
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].AtMS < cs[j].AtMS })
	return cs, nil
}

// Commits returns the author times (unix ms, ascending) of the non-merge
// commits on any branch of repo in [from, to). When the repo sets
// user.email, only that author's commits count.
func Commits(ctx context.Context, run Runner, repo string, from, to int64) ([]int64, error) {
	out, err := logRaw(ctx, run, repo, from, to, "%at")
	if err != nil {
		return nil, err
	}
	var times []int64
	for _, line := range strings.Fields(string(out)) {
		s, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("git log: unexpected line %q", line)
		}
		if ms := s * 1000; ms >= from && ms < to {
			times = append(times, ms)
		}
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	return times, nil
}

func logRaw(ctx context.Context, run Runner, repo string, from, to int64, format string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	// -E: git reads --author as a basic regex by default, where the \+ that
	// QuoteMeta makes of a plus address means "one or more".
	args := []string{"log", "--all", "--no-merges", "-E", "--format=" + format,
		"--since=@" + strconv.FormatInt(from/1000, 10), "--until=@" + strconv.FormatInt((to-1)/1000, 10)}
	if out, err := run(ctx, repo, "config", "user.email"); err == nil {
		if email := strings.TrimSpace(string(out)); email != "" {
			args = append(args, "--author=<"+regexp.QuoteMeta(email)+">")
		}
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return run(ctx, repo, args...)
}

// Check reports whether repo is a git repository git can read.
func Check(ctx context.Context, run Runner, repo string) error {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	_, err := run(ctx, repo, "rev-parse", "--git-dir")
	return err
}
