// Package classify labels spans with a category and project from rule files,
// groups unlabelled activity into an inbox and proposes learned rules. It is
// pure Go: no cgo, no database, and file I/O only when loading or writing rule
// files.
package classify

import (
	"regexp"
	"strings"
)

// SpanView is the part of a span classification needs. End is the clipped
// end; callers pass "now" for the open span.
type SpanView struct {
	ID, Start, End int64
	BundleID, App  string
	Title, URL     *string
	Private        bool
	Dir            *string // the shell's working directory, for terminal views; "~/…" under home
	InCall         bool    // a camera or microphone was in use (see MarkCalls)
	Kept           *Kept   // the label kept when the title was deleted; nil = none
	Planned        string  // the module of the plan's task at this time (see MarkPlan); "" = none
}

// DurMS returns End-Start, or 0 when End is before Start.
func (v SpanView) DurMS() int64 {
	if v.End < v.Start {
		return 0
	}
	return v.End - v.Start
}

// isNoiseRune reports runes that only carry status, never meaning: spinners,
// dots, stars and emoji. Box drawing (U+2500–257F) is deliberately absent: the
// WezTerm title format uses │ as a separator.
func isNoiseRune(r rune) bool {
	switch {
	case r >= 0x2800 && r <= 0x28FF, // Braille: terminal spinners
		r >= 0x2700 && r <= 0x27BF,            // Dingbats: ✳ ✻ ✶ ✢ ✽
		r >= 0x25A0 && r <= 0x25FF,            // Geometric shapes: ◐ ● ○ ◉
		r >= 0x2600 && r <= 0x26FF,            // Miscellaneous symbols
		r >= 0x1F300 && r <= 0x1FAFF,          // emoji
		r == 0x23FA, r == 0x2022, r == 0xFE0F: // ⏺ • variation selector
		return true
	}
	return false
}

func isNoiseToken(tok string) bool {
	if tok == "" {
		return false
	}
	for _, r := range tok {
		if !isNoiseRune(r) {
			return false
		}
	}
	return true
}

var (
	counterRe    = regexp.MustCompile(`^(\(\d+\+?\)|\[\d+/\d+\])$`)
	editSuffixRe = regexp.MustCompile(`(^|\s+)(—|-)\s*Edited$|(^|\s+)\[Modified\]$`)
)

func cleanOnce(s string) string {
	fields := strings.Fields(s)
	kept := fields[:0]
	for _, f := range fields {
		if isNoiseToken(f) || counterRe.MatchString(f) {
			continue
		}
		kept = append(kept, f)
	}
	s = strings.Join(kept, " ")
	s = editSuffixRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(strings.TrimPrefix(s, "*"), "*")
	return strings.TrimSpace(s)
}

// Clean removes status noise from a window title: standalone spinner, dot and
// emoji tokens, counters like "(3)" and "[1/3]", edit markers like
// " — Edited" and unsaved "*", and repeated whitespace. It never removes
// words or digits, so Clean(Clean(x)) == Clean(x) and distinct documents stay
// distinct. trackd uses it as the span merger's title key.
func Clean(title string) string {
	s := title
	for {
		next := cleanOnce(s)
		if next == s {
			return s
		}
		s = next
	}
}

// Key groups spans for the inbox: "<app> (private)", "<app> · <host>/<seg>",
// "<app> · <normalised title>" or "<app>".
func Key(v SpanView) string {
	switch {
	case v.Private:
		return v.App + " (private)"
	case v.URL != nil && *v.URL != "":
		return v.App + " · " + urlKey(*v.URL)
	case v.Title != nil:
		if t := titleKey(v.App, *v.Title); t != "" {
			return v.App + " · " + t
		}
	}
	return v.App
}

// urlKey reduces a stored host+path to host plus first path segment, without
// a leading "www.".
func urlKey(u string) string {
	u = strings.TrimPrefix(u, "www.")
	host, path, _ := strings.Cut(u, "/")
	seg, _, _ := strings.Cut(path, "/")
	if seg == "" {
		return host
	}
	return host + "/" + seg
}

var (
	digitsRe  = regexp.MustCompile(`[0-9]+`)
	versionRe = regexp.MustCompile(`^v?[0-9][0-9.]*$`)
)

// stripAppSuffix removes a trailing " - <app>" or " — <app>", optionally
// followed by a version ("- Obsidian 1.14.4"). s is already Clean, so
// separators are single spaces. Plain string compares keep arbitrary app
// names (even invalid UTF-8) safe.
func stripAppSuffix(s, app string) string {
	bases := []string{s}
	if i := strings.LastIndexByte(s, ' '); i >= 0 && versionRe.MatchString(s[i+1:]) {
		bases = append(bases, s[:i])
	}
	for _, base := range bases {
		for _, sep := range []string{" - ", " — "} {
			suf := sep + app
			if len(base) > len(suf) && strings.EqualFold(base[len(base)-len(suf):], suf) {
				return base[:len(base)-len(suf)]
			}
		}
	}
	return s
}

func isPathLike(tok string) bool {
	if strings.HasPrefix(tok, "~/") {
		return true
	}
	return strings.Count(strings.Trim(tok, "/"), "/") >= 1
}

func pathBase(tok string) string {
	t := strings.TrimRight(tok, "/")
	if i := strings.LastIndex(t, "/"); i >= 0 && i < len(t)-1 {
		return t[i+1:]
	}
	return tok
}

func titleKey(app, title string) string {
	s := Clean(title)
	if app != "" {
		s = stripAppSuffix(s, app)
	}
	f := strings.Fields(s)
	for i, t := range f {
		if isPathLike(t) {
			f[i] = pathBase(t)
		}
	}
	s = digitsRe.ReplaceAllString(strings.Join(f, " "), "#")
	return strings.ToLower(s)
}
