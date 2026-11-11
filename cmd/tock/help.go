package main

import (
	"fmt"
	"slices"
	"strings"
)

// wantsHelp reports whether command name's arguments ask for its help:
// "help" right after the command, or -h, -help or --help before a "--".
// "help" later on is an argument (a category, a note).
func wantsHelp(name string, args []string) bool {
	if freeText[name] {
		n := len(args)
		switch {
		case n == 0 || n > 2 || (name == "ask" && n != 1):
			return false
		case n == 1:
			return args[0] == "help" || isHelpFlag(args[0])
		}
		return args[0] != "--" && isHelpFlag(args[1])
	}
	if len(args) > 0 && args[0] == "help" {
		return true
	}
	for _, a := range args {
		if a == "--" {
			return false
		}
		if isHelpFlag(a) {
			return true
		}
	}
	return false
}

// ownUsage is each command's own usage text, where it has one: fuller than
// its lines in usage.
var ownUsage = map[string]string{
	"away":     awayUsage,
	"category": categoryUsage,
	"edit":     editUsage,
	"goal":     goalUsage,
	"invoice":  invoiceUsage,
	"model":    modelUsage,
	"project":  projectUsage,
	"rules":    rulesUsage,
	"top":      topUsage,
}

// subHelp is ownUsage[name] as help: "usage:" first, without the
// "name: " prefix some of them carry. "" when name has none.
func subHelp(name string) string {
	u, ok := ownUsage[name]
	if !ok {
		return ""
	}
	lines := strings.Split(strings.TrimRight(u, "\n"), "\n")
	// The first "tock" sets the indent the other lines were aligned to.
	first := 0
	for i, l := range lines {
		if strings.Contains(l, "tock ") {
			first = i
			break
		}
	}
	indent := strings.Index(lines[first], "tock ")
	var b strings.Builder
	b.WriteString("usage:\n")
	for _, l := range lines[first:] {
		if strings.Contains(l, "tock "+name) && strings.Index(l, "tock ") == indent {
			l = l[indent:]
		} else {
			lead := len(l) - len(strings.TrimLeft(l, " "))
			l = l[min(lead, indent):]
		}
		b.WriteString("  " + l + "\n")
	}
	return b.String()
}

// freeText commands take words of yours (a question, a note). For ask only
// a lone help word or flag asks for help (tock ask help me with -h); for
// session and edit only one that comes first or right after the
// subcommand, and last (tock session note --help, tock session list -h),
// so a note that says -h stays a note (tock session note 4 -h).
var freeText = map[string]bool{"ask": true, "session": true, "edit": true}

// usageLines are the lines of usage about command name: each line that
// starts with "  tock <name>" and the indented lines that continue it.
// "" when usage doesn't list it.
func usageLines(name string) string {
	var b strings.Builder
	in := false
	for line := range strings.Lines(usage) {
		switch {
		case strings.HasPrefix(line, "  tock "):
			rest := strings.TrimPrefix(line, "  tock ")
			in = rest == name+"\n" || strings.HasPrefix(rest, name+" ")
		case !strings.HasPrefix(line, "   "):
			in = false
		}
		if in {
			b.WriteString(line)
		}
	}
	return b.String()
}

// commandHelp is what `tock <name> --help` prints.
func commandHelp(name string) string {
	lines := subHelp(name)
	if lines == "" {
		if l := usageLines(name); l != "" {
			lines = "usage:\n" + l
		}
	}
	if lines == "" {
		return usage
	}
	return lines + "\ntock help lists every command; docs/guide.md explains each one.\n"
}

// helpFor is the help shown after a usage error in command name: its own
// help, unless the error already is that (tock project frob), or all of
// usage when it has none.
func helpFor(name string, err error) string {
	if u, ok := ownUsage[name]; ok && strings.Contains(err.Error(), strings.TrimSpace(u)) {
		return ""
	}
	if slices.Contains([]string{"", "help"}, name) {
		return usage
	}
	return commandHelp(name)
}

// views are names people try for what the TUI and dashboard show.
var views = map[string]string{
	"week":      "the week is in the terminal UI (tock) and the dashboard (tock web)",
	"month":     "the month is in the dashboard (tock web)",
	"day":       "the day is tock today, or the terminal UI (tock)",
	"dashboard": "the dashboard is tock web",
	"stats":     "tock top, tock trends and tock rules stats show where the time went",
}

// unknownHelp is what follows "unknown command": a near command, or where
// the view lives, or the pointer to the full list.
func unknownHelp(name string) string {
	if v, ok := views[name]; ok {
		return v + "; tock help lists every command\n"
	}
	best, bestD := "", 3
	for c := range commands {
		d := editDistance(name, c)
		if strings.HasPrefix(c, name) || strings.HasPrefix(name, c) {
			d = min(d, 1)
		}
		if d < bestD || (d == bestD && c < best) {
			best, bestD = c, d
		}
	}
	if best != "" && len(name) > 2 {
		return fmt.Sprintf("did you mean %q? tock help lists every command\n", best)
	}
	return "tock help lists every command\n"
}

// editDistance is the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func isHelpFlag(a string) bool { return a == "-h" || a == "-help" || a == "--help" }
