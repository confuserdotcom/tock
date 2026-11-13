// Package testfs holds file checks shared by tests on every platform.
package testfs

import (
	"os"
	"runtime"
)

// Perm reports whether fi has the Unix permission bits want. Windows has no
// such bits (Go reports 0666 or 0444 from the read-only attribute; files
// under the profile are private by their ACL), so it is always true there.
func Perm(fi os.FileInfo, want os.FileMode) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return fi != nil && fi.Mode().Perm() == want
}

// TB is the part of testing.TB that UnixOnly needs.
type TB interface {
	Helper()
	Skip(args ...any)
}

// UnixOnly skips a test of behaviour that exists only on Unix, saying why.
func UnixOnly(t TB, why string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix only: " + why)
	}
}
