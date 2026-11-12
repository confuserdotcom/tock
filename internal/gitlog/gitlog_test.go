package gitlog

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fake struct {
	calls [][]string
	email string
	log   string
	err   error
}

func (f *fake) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{dir}, args...))
	if args[0] == "config" {
		if f.email == "" {
			return nil, errors.New("exit status 1")
		}
		return []byte(f.email + "\n"), nil
	}
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.log), nil
}

func TestCommitsArgs(t *testing.T) {
	f := &fake{email: "me+x@example.com", log: "1700000300\n1700000100\n1699999999\n1700000400\n"}
	got, err := Commits(context.Background(), f.run, "/r", 1_700_000_000_000, 1_700_000_400_000)
	if err != nil {
		t.Fatal(err)
	}
	// 1699999999 is before from and 1700000400 is at to: both dropped. Sorted.
	if want := []int64{1_700_000_100_000, 1_700_000_300_000}; !reflect.DeepEqual(got, want) {
		t.Fatalf("times = %v, want %v", got, want)
	}
	want := []string{"/r", "log", "--all", "--no-merges", "-E", "--format=%at", "--since=@1700000000",
		"--until=@1700000399", `--author=<me\+x@example\.com>`}
	if !reflect.DeepEqual(f.calls[1], want) {
		t.Fatalf("args = %q\nwant %q", f.calls[1], want)
	}
}

func TestCommitsNoEmailAndErrors(t *testing.T) {
	f := &fake{log: "1700000100\n"}
	if _, err := Commits(context.Background(), f.run, "/r", 0, 2_000_000_000_000); err != nil {
		t.Fatal(err)
	}
	if last := f.calls[1]; strings.HasPrefix(last[len(last)-1], "--author") {
		t.Fatalf("no email must mean no author filter: %q", last)
	}
	f = &fake{err: errors.New("not a git repository")}
	if _, err := Commits(context.Background(), f.run, "/r", 0, 1); err == nil {
		t.Fatal("want the log error")
	}
	f = &fake{log: "garbage\n"}
	if _, err := Commits(context.Background(), f.run, "/r", 0, 1); err == nil {
		t.Fatal("want a parse error")
	}
	slow := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := Commits(ctx, slow, "/r", 0, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout err = %v", err)
	}
}

// TestRealGit builds a throwaway repo with synthetic commits.
func TestRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git := func(env []string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), append([]string{"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null"}, env...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(nil, "init", "-q", "-b", "main")
	git(nil, "config", "user.email", "1+me@example.com") // a plus, as GitHub noreply addresses have
	git(nil, "config", "user.name", "Me")
	commit := func(at int64, author string) {
		d := time.Unix(at, 0).UTC().Format(time.RFC3339)
		env := []string{"GIT_AUTHOR_DATE=" + d, "GIT_COMMITTER_DATE=" + d}
		if author != "" {
			env = append(env, "GIT_AUTHOR_EMAIL="+author)
		}
		if err := os.WriteFile(filepath.Join(dir, "f"), []byte(d+author), 0o600); err != nil {
			t.Fatal(err)
		}
		git(env, "add", "f")
		git(env, "commit", "-q", "-m", "x")
	}
	commit(1_700_000_100, "")
	commit(1_700_000_200, "other@example.com")
	commit(1_700_000_300, "")
	commit(1_700_100_000, "")

	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	got, err := Commits(context.Background(), Exec, dir, 1_700_000_000_000, 1_700_050_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{1_700_000_100_000, 1_700_000_300_000}; !reflect.DeepEqual(got, want) {
		t.Fatalf("times = %v, want %v", got, want)
	}
	log, err := Log(context.Background(), Exec, dir, 1_700_000_000_000, 1_700_050_000_000)
	if err != nil || len(log) != 2 || log[0].Subject != "x" || len(log[0].Hash) < 7 {
		t.Fatalf("log = %+v, %v", log, err)
	}
	if err := Check(context.Background(), Exec, dir); err != nil {
		t.Fatalf("Check(repo) = %v", err)
	}
	if err := Check(context.Background(), Exec, t.TempDir()); err == nil {
		t.Fatal("Check(plain dir) = nil")
	}
}

func TestLog(t *testing.T) {
	f := &fake{email: "me@x.io", log: "1700000300\x1fabc1234\x1fFix the thing\n1700000100\x1fdef5678\x1fFirst\x1fwith sep\n1699999999\x1f0000000\x1fold\n"}
	got, err := Log(context.Background(), f.run, "/r", 1_700_000_000_000, 1_700_000_400_000)
	if err != nil {
		t.Fatal(err)
	}
	want := []Commit{
		{AtMS: 1_700_000_100_000, Hash: "def5678", Subject: "First\x1fwith sep"},
		{AtMS: 1_700_000_300_000, Hash: "abc1234", Subject: "Fix the thing"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	args := f.calls[1]
	if args[5] != "--format=%at%x1f%h%x1f%s" || args[len(args)-1] != `--author=<me@x\.io>` {
		t.Fatalf("args = %q", args)
	}
	if _, err := Log(context.Background(), (&fake{log: "nope\n"}).run, "/r", 0, 1); err == nil {
		t.Fatal("bad line must fail")
	}
}
