package classify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const learnedHeader = `# tock learned rules: written by ` + "`tock tag`" + `, checked before rules.toml.
# Newest first. Move a rule into rules.toml to own it, or delete it.

`

// tomlString renders s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// renderRule renders r as a [[rule]] table preceded by a comment line.
func renderRule(r Rule, comment string) string {
	var b strings.Builder
	comment = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, comment)
	fmt.Fprintf(&b, "# %s\n[[rule]]\n", comment)
	if len(r.Bundle) > 0 {
		q := make([]string, len(r.Bundle))
		for i, x := range r.Bundle {
			q[i] = tomlString(x)
		}
		fmt.Fprintf(&b, "bundle = [%s]\n", strings.Join(q, ", "))
	}
	for _, f := range []struct{ k, v string }{
		{"app", r.App}, {"title", r.Title}, {"url", r.URL}, {"dir", r.Dir},
		{"category", r.Category}, {"project", r.Project},
	} {
		if f.v != "" {
			fmt.Fprintf(&b, "%s = %s\n", f.k, tomlString(f.v))
		}
	}
	if r.SinceMS != 0 {
		fmt.Fprintf(&b, "since = %s\n", SinceText(r.SinceMS))
	}
	return b.String() + "\n"
}

// SinceShort is ms as people read it: "2026-11-20 14:05", local time.
func SinceShort(ms int64) string {
	return time.UnixMilli(ms).In(time.Local).Format("2006-01-02 15:04")
}

// SinceText is ms as a rule's since value: a TOML local datetime in the
// local zone, to the second.
func SinceText(ms int64) string {
	return time.UnixMilli(ms).In(time.Local).Format("2006-01-02T15:04:05")
}

// AppendLearnedText is old, the learned file's content, with r added as
// its newest (first) rule. Content that does not parse is an error.
func AppendLearnedText(old []byte, r Rule, comment string) ([]byte, error) {
	var lf learnedTOML
	if err := decodeStrict(LearnedFile, old, &lf); err != nil {
		return nil, fmt.Errorf("%w (not modified)", err)
	}
	block := renderRule(r, comment)
	text := string(old)
	var out string
	switch i := strings.Index(text, "[[rule]]"); {
	case len(strings.TrimSpace(text)) == 0:
		out = learnedHeader + block
	case i < 0:
		out = strings.TrimRight(text, "\n") + "\n\n" + block
	default:
		// Insert before the comment lines directly above the first rule.
		start := strings.LastIndex(text[:i], "\n\n")
		if start < 0 {
			start = 0
		} else {
			start += 2
		}
		out = text[:start] + block + text[start:]
	}
	return []byte(out), nil
}

// WriteFile replaces the rule file at path atomically (see writeAtomic).
func WriteFile(path string, data []byte) error {
	return writeAtomic(path, "."+strings.TrimSuffix(filepath.Base(path), ".toml")+"-*.toml", data)
}

// writeAtomic replaces path with data: a 0600 temp file in the same 0700
// directory, synced, then renamed over path.
func writeAtomic(path, pattern string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
