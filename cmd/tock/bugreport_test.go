package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tock/internal/testfs"
)

const sentinel = "ZZSENTINELZZ"

// No title, URL, name, address, sync folder or hidden site from the
// user's data reaches the archive.
func TestBugReportLeaksNothing(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t,
		seedSpan{1, "09:00", "10:00", "arc", "Arc", sentinel + " secret page", "https://" + sentinel + ".example/x", false},
		seedSpan{2, "10:00", "11:00", "term", "WezTerm", "vim " + sentinel, "", false},
	)
	syncDir := filepath.Join(t.TempDir(), sentinel+"-sync")
	cfg := `# my notes ` + sentinel + `
[privacy]
keep_titles = 30
hide_sites = [
  "` + sentinel + `.example",
]

[sync]
dir = "` + syncDir + `"

[invoice]
name = "` + sentinel + ` Ltd"
address = ["1 ` + sentinel + ` Street", "Town"]
`
	if err := os.WriteFile(filepath.Join(te.ConfigDir, "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(te.StateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	trackdLog := "level=INFO msg=\"trackd starting\"\nlevel=WARN msg=x title=\"" + sentinel + "\"\nlevel=INFO msg=y url=https://" + sentinel + ".example\nlevel=INFO msg=\"kept line\"\n"
	if err := os.WriteFile(filepath.Join(te.StateDir, "trackd.log"), []byte(trackdLog), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(te.StateDir, "web.log"), []byte("GET /?u=https://"+sentinel+".example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "report.tar.gz")
	code, stdout := te.run(t, "", "bug-report", "-o", out)
	if code != 0 {
		t.Fatalf("exit %d: %s %s", code, stdout, te.errb)
	}
	if !strings.Contains(stdout, "doctor.json") || !strings.Contains(stdout, "Nothing was sent") {
		t.Fatalf("stdout:\n%s", stdout)
	}
	if fi, err := os.Stat(out); err != nil || !testfs.Perm(fi, 0o600) {
		t.Fatalf("archive mode: %v %v", fi, err)
	}
	members := readTarGz(t, out)
	for _, want := range []string{"doctor.json", "config.toml", "trackd.log", "trackd_runs.json"} {
		if _, ok := members[want]; !ok {
			t.Errorf("archive lacks %s; has %v", want, keys(members))
		}
	}
	if _, ok := members["rules.toml"]; ok {
		t.Error("rules.toml without --include-rules")
	}
	for name, body := range members {
		if strings.Contains(body, sentinel) {
			t.Errorf("%s leaks the sentinel:\n%s", name, body)
		}
	}
	if !strings.Contains(members["trackd.log"], "kept line") {
		t.Errorf("trackd.log lost an ordinary line:\n%s", members["trackd.log"])
	}
	if !strings.Contains(members["config.toml"], "keep_titles = 30") || !strings.Contains(members["config.toml"], "[sync]") {
		t.Errorf("config.toml lost its shape:\n%s", members["config.toml"])
	}

	code, _ = te.run(t, "", "bug-report", "-o", out, "--include-rules")
	if _, ok := readTarGz(t, out)["rules.toml"]; code != 0 || !ok {
		t.Fatal("--include-rules did not add rules.toml")
	}
}

func readTarGz(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	out := map[string]string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		out[h.Name] = string(b)
	}
}

func keys(m map[string]string) []string {
	var k []string
	for n := range m {
		k = append(k, n)
	}
	return k
}
