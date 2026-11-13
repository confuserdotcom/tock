// Package docscheck fails the build when the docs drift from what tock
// supports: every platform named where a user decides whether to install
// or how to report a bug, no release version pinned in install commands,
// and no link to a doc that is not there.
package docscheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const root = "../.."

// platforms are what trackd records on; keep in step with
// internal/desktop and the collectors.
var platforms = []string{"macOS", "Hyprland", "Sway", "i3", "X11", "Windows"}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPlatformsNamed(t *testing.T) {
	for _, f := range []string{"README.md", ".github/ISSUE_TEMPLATE/bug.yml"} {
		text := read(t, f)
		for _, p := range platforms {
			if !strings.Contains(text, p) {
				t.Errorf("%s does not name %s", f, p)
			}
		}
	}
}

// A version in an install command goes stale with the next release.
var pinned = regexp.MustCompile(`tock-v[0-9]+\.[0-9]+\.[0-9]+-(linux|darwin|windows)`)

func TestNoPinnedVersion(t *testing.T) {
	for _, f := range []string{"README.md", "docs/guide.md", "docs/index.md", "docs/verify.md"} {
		if m := pinned.FindString(read(t, f)); m != "" {
			t.Errorf("%s pins %s; use a glob or <version>", f, m)
		}
	}
}

var link = regexp.MustCompile(`\]\(((?:docs/|\.\./|\./)?[A-Za-z0-9_./-]+\.md)(?:#[^)]*)?\)`)

func TestDocLinks(t *testing.T) {
	for _, f := range []string{"README.md", "SECURITY.md", "docs/guide.md", "docs/index.md", "docs/privacy.md", "docs/verify.md", "docs/security-review.md"} {
		for _, m := range link.FindAllStringSubmatch(read(t, f), -1) {
			target := filepath.Join(filepath.Dir(f), m[1])
			if _, err := os.Stat(filepath.Join(root, target)); err != nil {
				t.Errorf("%s links to %s, which does not exist", f, m[1])
			}
		}
	}
}
