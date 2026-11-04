package config

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

// SetBool returns src with key = v in [table]. It edits lines, so comments,
// order and every other setting stay as they were: the key's value is
// replaced where it is (a trailing comment stays), or the key is added
// after the table's last setting, or the table is added at the end. It
// fails when the result does not parse or does not read back v, for
// example when src sets the table inline or with dotted keys; src is then
// left to be edited by hand.
func SetBool(src []byte, table, key string, v bool) ([]byte, error) {
	val := fmt.Sprint(v)
	text := string(src)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	lines := strings.SplitAfter(text, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	start, end := -1, len(lines) // the table's lines: (start, end)
	for i, l := range lines {
		name, ok := header(l)
		if !ok {
			continue
		}
		if start >= 0 {
			end = i
			break
		}
		if name == table {
			start = i
		}
	}

	var out []string
	switch {
	case start < 0:
		out = append(out, lines...)
		if len(out) > 0 {
			out = append(out, "\n")
		}
		out = append(out, "["+table+"]\n", key+" = "+val+"\n")
	default:
		at, last := -1, start
		for i := start + 1; i < end; i++ {
			t := strings.TrimSpace(lines[i])
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			last = i
			if k, _, ok := strings.Cut(t, "="); ok && strings.TrimSpace(k) == key {
				at = i
			}
		}
		out = append(out, lines...)
		if at >= 0 {
			out[at] = replaceValue(lines[at], val)
		} else {
			out = append(out[:last+1:last+1], append([]string{key + " = " + val + "\n"}, lines[last+1:]...)...)
		}
	}
	b := []byte(strings.Join(out, ""))

	var m map[string]any
	if _, err := toml.Decode(string(b), &m); err != nil {
		return nil, fmt.Errorf("config: set [%s] %s: %w", table, key, err)
	}
	if t, ok := m[table].(map[string]any); !ok || t[key] != v {
		return nil, fmt.Errorf("config: set [%s] %s: the file sets it in a way tock cannot edit; change it by hand", table, key)
	}
	if bytes.Equal(b, src) {
		return src, nil
	}
	return b, nil
}

// header is the table name of a [table] line.
func header(l string) (string, bool) {
	t := strings.TrimSpace(l)
	if !strings.HasPrefix(t, "[") {
		return "", false
	}
	t = strings.TrimPrefix(t, "[")
	t = strings.TrimPrefix(t, "[") // [[array]] ends a table too
	name, _, ok := strings.Cut(t, "]")
	if !ok {
		return "", false
	}
	return strings.Trim(strings.TrimSpace(name), `"`), true
}

// replaceValue swaps the value of a key = value line, keeping its
// indentation, spacing and trailing comment.
func replaceValue(l, val string) string {
	eq := strings.Index(l, "=")
	rest := l[eq+1:]
	lead := len(rest) - len(strings.TrimLeft(rest, " \t"))
	body := rest[lead:]
	n := strings.IndexAny(body, " \t#\r\n")
	if n < 0 {
		n = len(body)
	}
	return l[:eq+1] + rest[:lead] + val + body[n:]
}
