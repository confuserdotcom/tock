package classify

import (
	"path/filepath"
	"runtime"
	"strings"
)

// foldPaths: Windows file names compare without regard to case.
var foldPaths = runtime.GOOS == "windows"

// pathKey is p cleaned, with / separators, and folded where case doesn't
// count, so the same folder written two ways compares equal.
func pathKey(p string) string {
	k := filepath.ToSlash(filepath.Clean(p))
	if foldPaths {
		k = strings.ToLower(k)
	}
	return k
}

// Within reports whether dir is root or a folder inside it.
func Within(dir, root string) bool {
	if dir == "" || root == "" {
		return false
	}
	d, r := pathKey(dir), pathKey(root)
	return d == r || strings.HasPrefix(d, strings.TrimSuffix(r, "/")+"/")
}
