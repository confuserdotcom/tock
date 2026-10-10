package report

import (
	"fmt"
	"sort"
	"strings"
)

// NameN is a name with a count.
type NameN struct {
	Name string
	N    int
}

// Commits lists the projects with at least one commit, most first, then by
// name.
func Commits(byProject map[string]int) []NameN {
	var out []NameN
	for name, n := range byProject {
		if n > 0 {
			out = append(out, NameN{name, n})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// CommitsLine is "tock 4 · ATLAS 1".
func CommitsLine(rows []NameN) string {
	parts := make([]string, len(rows))
	for i, r := range rows {
		parts[i] = fmt.Sprintf("%s %d", Safe(r.Name), r.N)
	}
	return strings.Join(parts, " · ")
}
