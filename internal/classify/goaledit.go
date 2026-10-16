package classify

import (
	"regexp"
	"strings"
)

var goalStart = regexp.MustCompile(`^\s*\[\[\s*goal\s*\]\]\s*(#.*)?$`)

// goalSpans finds each [[goal]] table in file order: its header line and
// the line after its last key line.
func goalSpans(lines []string) [][2]int {
	var out [][2]int
	for n, l := range lines {
		if !goalStart.MatchString(strings.TrimRight(l, "\r\n")) {
			continue
		}
		end := n + 1
		for end < len(lines) && !isBlank(lines[end]) && !tableHeader.MatchString(lines[end]) {
			end++
		}
		out = append(out, [2]int{n, end})
	}
	return out
}

// goalTable is g as a [[goal]] table.
func goalTable(g Goal) string {
	var b strings.Builder
	b.WriteString("[[goal]]\n")
	if g.Category != "" {
		b.WriteString("category = " + tomlString(g.Category) + "\n")
	}
	if g.Project != "" {
		b.WriteString("project = " + tomlString(g.Project) + "\n")
	}
	bound := "at_least"
	if g.AtMost {
		bound = "at_most"
	}
	b.WriteString(bound + " = " + tomlString(FormatDur(g.MS)) + "\n")
	if g.Week {
		b.WriteString("per = \"week\"\n")
	}
	return b.String()
}

// SameGoal reports whether a and b count the same label the same way (at
// least or at most, per day or week), whatever their time.
func SameGoal(a, b Goal) bool {
	return a.Category == b.Category && a.Project == b.Project && a.AtMost == b.AtMost && a.Week == b.Week
}

// SetGoal rewrites the rules.toml text so it holds goal g: the goal of
// goals (the ruleset's, parsed from text) that SameGoal matches takes g's
// time, else g is added after the last goal. g.MS 0 removes the match.
func SetGoal(text string, goals []Goal, g Goal) string {
	lines := strings.SplitAfter(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	spans := goalSpans(lines)
	for _, old := range goals {
		if !SameGoal(old, g) || old.Index > len(spans) {
			continue
		}
		sp := spans[old.Index-1]
		lo, hi := sp[0], sp[1]
		if g.MS == 0 {
			if hi < len(lines) && isBlank(lines[hi]) {
				hi++
			} else if lo > 0 && isBlank(lines[lo-1]) {
				lo--
			}
			return strings.Join(lines[:lo], "") + strings.Join(lines[hi:], "")
		}
		return strings.Join(lines[:lo], "") + goalTable(g) + strings.Join(lines[hi:], "")
	}
	if g.MS == 0 {
		return text
	}
	at := len(lines)
	if n := len(spans); n > 0 {
		at = spans[n-1][1]
	}
	head := strings.Join(lines[:at], "")
	if head != "" && !strings.HasSuffix(head, "\n") {
		head += "\n"
	}
	if head != "" && !strings.HasSuffix(head, "\n\n") {
		head += "\n"
	}
	tail := strings.Join(lines[at:], "")
	if tail != "" && !strings.HasPrefix(tail, "\n") {
		tail = "\n" + tail
	}
	return head + goalTable(g) + tail
}
