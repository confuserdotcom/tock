// Package gitlog reads commit times from local git repositories. It reads
// author times only: never subjects, hashes or paths, and it never fetches.
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

// Commits returns the author times (unix ms, ascending) of the non-merge
// commits on any branch of repo in [from, to). When the repo sets
// user.email, only that author's commits count.
func Commits(ctx context.Context, run Runner, repo string, from, to int64) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	args := []string{"log", "--all", "--no-merges", "--format=%at",
		"--since=@" + strconv.FormatInt(from/1000, 10), "--until=@" + strconv.FormatInt((to-1)/1000, 10)}
	if out, err := run(ctx, repo, "config", "user.email"); err == nil {
		if email := strings.TrimSpace(string(out)); email != "" {
			args = append(args, "--author=<"+regexp.QuoteMeta(email)+">")
		}
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	out, err := run(ctx, repo, args...)
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

// Check reports whether repo is a git repository git can read.
func Check(ctx context.Context, run Runner, repo string) error {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	_, err := run(ctx, repo, "rev-parse", "--git-dir")
	return err
}
