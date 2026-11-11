package main

import (
	"fmt"
	"slices"
	"strings"
)

// wantsHelp reports whether a command's arguments ask for its help: "help"
// right after the command, or -h, -help or --help before a "--". "help"
// later on is an argument (a category, a note).
func wantsHelp(args []string) bool {
	if len(args) > 0 && args[0] == "help" {
		return true
	}
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "-h" || a == "-help" || a == "--help" {
			return true
		}
	}
	return false
}

// subHelp is the fuller help of commands with subcommands of their own.
var subHelp = map[string]string{
	"invoice": strings.TrimPrefix(invoiceUsage, "invoice: one of\n") + "\n",
}

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
	lines := usageLines(name)
	if extra, ok := subHelp[name]; ok {
		lines = extra
	}
	if lines == "" {
		return usage
	}
	return "usage:\n" + lines + "\ntock help lists every command; docs/guide.md explains each one.\n"
}

// helpFor is the help shown after a usage error in command name: its own
// lines, or all of usage when it has none.
func helpFor(name string, err error) string {
	if extra, ok := subHelp[name]; ok && strings.Contains(err.Error(), strings.TrimSpace(extra)) {
		return "" // the error already is the command's help (tock invoice frob)
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
