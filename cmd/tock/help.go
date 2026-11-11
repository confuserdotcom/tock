package main

import (
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
