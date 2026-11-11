package config

import (
	"bytes"
	"fmt"
	"strconv"
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
	b := setValue(src, table, key, fmt.Sprint(v))
	return check(src, b, table, key, func(got any) bool { return got == v })
}

// Sets reports whether src sets key in [table] itself, whatever its value;
// a file that does not parse sets nothing.
func Sets(src []byte, table, key string) bool {
	var m map[string]any
	if _, err := toml.Decode(string(src), &m); err != nil {
		return false
	}
	t, ok := m[table].(map[string]any)
	if !ok {
		return false
	}
	_, ok = t[key]
	return ok
}

// SetList is SetBool for a list of strings: the key's value, on one line or
// several, becomes a one-line array.
func SetList(src []byte, table, key string, vals []string) ([]byte, error) {
	q := make([]string, len(vals))
	for i, v := range vals {
		q[i] = strconv.Quote(v)
	}
	b := setValue(src, table, key, "["+strings.Join(q, ", ")+"]")
	return check(src, b, table, key, func(got any) bool {
		l, ok := got.([]any)
		if !ok || len(l) != len(vals) {
			return false
		}
		for i := range l {
			if l[i] != vals[i] {
				return false
			}
		}
		return true
	})
}

// check fails when b does not parse or [table] key does not read back as
// want says; b equal to src returns src.
func check(src, b []byte, table, key string, want func(any) bool) ([]byte, error) {
	var m map[string]any
	if _, err := toml.Decode(string(b), &m); err != nil {
		return nil, fmt.Errorf("config: set [%s] %s: change it by hand: %w", table, key, err)
	}
	if t, ok := m[table].(map[string]any); !ok || !want(t[key]) {
		return nil, fmt.Errorf("config: set [%s] %s: the file sets it in a way tock cannot edit; change it by hand", table, key)
	}
	if bytes.Equal(b, src) {
		return src, nil
	}
	return b, nil
}

// setValue edits the lines of src so [table] key = val.
func setValue(src []byte, table, key, val string) []byte {
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
		at, upto, last := -1, -1, start
		for i := start + 1; i < end; i++ {
			t := strings.TrimSpace(lines[i])
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			last = i
			k, v, ok := strings.Cut(t, "=")
			if !ok || strings.TrimSpace(k) != key {
				continue
			}
			at, upto = i, i
			for d := depth(v); d > 0 && upto+1 < end; {
				upto++
				d += depth(lines[upto])
			}
			last = upto
			i = upto
		}
		switch {
		case at >= 0 && upto == at:
			out = append(out, lines...)
			if _, v, _ := strings.Cut(lines[at], "="); strings.HasPrefix(strings.TrimSpace(v), "[") {
				out[at] = replaceArray(lines[at], val)
			} else {
				out[at] = replaceValue(lines[at], val)
			}
		case at >= 0:
			indent := lines[at][:len(lines[at])-len(strings.TrimLeft(lines[at], " \t"))]
			out = append(out, lines[:at]...)
			out = append(out, indent+key+" = "+val+"\n")
			out = append(out, lines[upto+1:]...)
		default:
			out = append(out, lines[:last+1]...)
			out = append(out, key+" = "+val+"\n")
			out = append(out, lines[last+1:]...)
		}
	}
	return []byte(strings.Join(out, ""))
}

// replaceArray is replaceValue for a one-line array value.
func replaceArray(l, val string) string {
	eq := strings.Index(l, "=")
	rest := l[eq+1:]
	lead := len(rest) - len(strings.TrimLeft(rest, " \t"))
	body := rest[lead:]
	n := closeAt(body)
	return l[:eq+1] + rest[:lead] + val + body[n:]
}

// closeAt is the index just past the ] that closes the array s opens, or
// len(s).
func closeAt(s string) int {
	n := len(s)
	scan(s, func(i, d int) bool {
		if d == 0 {
			n = i + 1
			return false
		}
		return true
	})
	return n
}

// depth is how many more [ than ] a line opens, outside strings and
// comments.
func depth(l string) int {
	d := 0
	scan(l, func(_, n int) bool { d = n; return true })
	return d
}

// scan calls f(i, depth) after each [ or ] of l outside strings, until a
// comment, the end, or f returns false.
func scan(l string, f func(i, depth int) bool) {
	d := 0
	var quote byte
	esc := false
	for i := 0; i < len(l); i++ {
		c := l[i]
		switch {
		case quote != 0:
			switch {
			case esc:
				esc = false
			case c == '\\' && quote == '"':
				esc = true
			case c == quote:
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			return
		case c == '[' || c == ']':
			if c == '[' {
				d++
			} else {
				d--
			}
			if !f(i, d) {
				return
			}
		}
	}
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

// UnsetKey returns src without key in [table], editing lines the way SetBool
// does. A table left with no lines at all goes too, with the blank line
// SetBool puts before a table it adds. It fails when the result does not
// parse or still sets the key (a dotted key or an inline table).
func UnsetKey(src []byte, table, key string) ([]byte, error) {
	text := string(src)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	lines := strings.SplitAfter(text, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	start, end := -1, len(lines)
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
	out := lines
	if start >= 0 {
		at, upto := -1, -1
		for i := start + 1; i < end; i++ {
			k, v, ok := strings.Cut(strings.TrimSpace(lines[i]), "=")
			if !ok || strings.TrimSpace(k) != key {
				continue
			}
			at, upto = i, i
			for d := depth(v); d > 0 && upto+1 < end; {
				upto++
				d += depth(lines[upto])
			}
			break
		}
		if at >= 0 {
			out = append(append([]string{}, lines[:at]...), lines[upto+1:]...)
			end -= upto + 1 - at
			if end == start+1 { // nothing left under the header
				from := start
				if from > 0 && strings.TrimSpace(out[from-1]) == "" {
					from--
				}
				out = append(out[:from:from], out[start+1:]...)
			}
		}
	}
	b := []byte(strings.Join(out, ""))
	if len(out) == len(lines) {
		b = src
	}
	var m map[string]any
	if _, err := toml.Decode(string(b), &m); err != nil {
		return nil, fmt.Errorf("config: unset [%s] %s: change it by hand: %w", table, key, err)
	}
	if Sets(b, table, key) {
		return nil, fmt.Errorf("config: unset [%s] %s: the file sets it in a way tock cannot edit; change it by hand", table, key)
	}
	return b, nil
}
