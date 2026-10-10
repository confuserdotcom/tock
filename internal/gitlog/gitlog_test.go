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
	want := []string{"/r", "log", "--all", "--no-merges", "--format=%at", "--since=@1700000000",
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
	git(nil, "config", "user.email", "me@example.com")
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
	if err := Check(context.Background(), Exec, dir); err != nil {
		t.Fatalf("Check(repo) = %v", err)
	}
	if err := Check(context.Background(), Exec, t.TempDir()); err == nil {
		t.Fatal("Check(plain dir) = nil")
	}
}
