package rulesfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"tock/internal/testfs"
)

const rules = "[[category]]\nname = \"Study\"\nkind = \"deep\"\n"

func files(t *testing.T) Files {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "config")
	f := Files{Rules: filepath.Join(dir, "rules.toml"), Learned: filepath.Join(dir, "learned.toml")}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.Rules, []byte(rules), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return "<" + err.Error() + ">"
	}
	return string(b)
}

func TestReadWrite(t *testing.T) {
	f := files(t)
	cur, err := f.Read()
	if err != nil || string(cur.Rules) != rules || cur.Learned != nil || cur.HasLearned {
		t.Fatalf("read = %+v, %v", cur, err)
	}
	next := cur
	next.Rules = []byte(rules + "\n[[category]]\nname = \"Code\"\nkind = \"deep\"\n")
	next.Learned = []byte("[[rule]]\nbundle = [\"x\"]\ncategory = \"Code\"\n")
	var order []string
	at := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	err = f.Write(cur, next, Options{At: at, BeforeRules: func() error {
		order = append(order, "learned="+read(t, f.Learned), "rules="+read(t, f.Rules))
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, f.Rules) != string(next.Rules) || read(t, f.Learned) != string(next.Learned) {
		t.Fatal("files not written")
	}
	if read(t, f.Rules+".bak") != rules {
		t.Fatalf("rules.toml.bak = %q", read(t, f.Rules+".bak"))
	}
	if _, err := os.Stat(f.Learned + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("no learned.toml before: no .bak")
	}
	// BeforeRules runs once learned.toml is written and rules.toml is not.
	if len(order) != 2 || order[0] != "learned="+string(next.Learned) || order[1] != "rules="+rules {
		t.Fatalf("BeforeRules saw %q", order)
	}
	if fi, _ := os.Stat(f.Rules); !fi.ModTime().Equal(at) {
		t.Fatalf("rules.toml changed at %v, want %v", fi.ModTime(), at)
	}
	if fi, _ := os.Stat(f.Rules); !testfs.Perm(fi, 0o600) {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
}

// Rules that do not parse are never written.
func TestWriteRefusesBrokenRules(t *testing.T) {
	f := files(t)
	cur, _ := f.Read()
	next := cur
	next.Rules = []byte("[[category]]\nname = \n")
	if err := f.Write(cur, next, Options{}); err == nil || !strings.Contains(err.Error(), "nothing changed") {
		t.Fatalf("err = %v", err)
	}
	if read(t, f.Rules) != rules {
		t.Fatal("broken rules were written")
	}
	// A learned rule naming an unknown category is broken too.
	next = cur
	next.Learned = []byte("[[rule]]\nbundle = [\"x\"]\ncategory = \"Nope\"\n")
	if err := f.Write(cur, next, Options{}); err == nil {
		t.Fatal("learned rules with an unknown category must be refused")
	}
	if _, err := os.Stat(f.Learned); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("refused learned.toml was written")
	}
}

// Write refuses when the files changed since prev was read: another
// writer's change is never lost.
func TestWriteRefusesStale(t *testing.T) {
	f := files(t)
	cur, _ := f.Read()
	if err := os.WriteFile(f.Rules, []byte(rules+"# edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	next := cur
	next.Rules = []byte(rules + "\n")
	if err := f.Write(cur, next, Options{}); !errors.Is(err, ErrChanged) {
		t.Fatalf("err = %v, want ErrChanged", err)
	}
}

// Rewrite holds the lock: concurrent rewrites each see the other's result.
func TestRewriteSerializes(t *testing.T) {
	f := files(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := f.Rewrite(func(cur Set) (Set, error) {
				cur.Rules = append(append([]byte{}, cur.Rules...), []byte("# "+strings.Repeat("x", i+1)+"\n")...)
				return cur, nil
			}, Options{})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if n := strings.Count(read(t, f.Rules), "# x"); n != 8 {
		t.Fatalf("%d of 8 rewrites kept:\n%s", n, read(t, f.Rules))
	}
}

// Restore puts back a file as an undo has it, or removes one that did not
// exist, and refuses when the file changed since.
func TestRestore(t *testing.T) {
	f := files(t)
	if err := f.Restore(f.Rules, true, []byte("old"), []byte("not what is there")); !errors.Is(err, ErrChanged) {
		t.Fatalf("err = %v", err)
	}
	if err := f.Restore(f.Rules, true, []byte(rules+"# old\n"), []byte(rules)); err != nil {
		t.Fatal(err)
	}
	if read(t, f.Rules) != rules+"# old\n" {
		t.Fatal("not restored")
	}
	os.WriteFile(f.Learned, []byte("x"), 0o600)
	if err := f.Restore(f.Learned, false, nil, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.Learned); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a file that did not exist must be removed")
	}
}
