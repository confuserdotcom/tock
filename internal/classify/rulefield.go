package classify

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
)

// SetRuleField sets key of rule i (0-based, = Rule.Index-1) in the text of one
// rule file to value (TOML text, e.g. "false"), replacing its line or adding
// one directly after the [[rule]] header; value "" removes the line. Every
// other byte of the file is kept. The result must decode to the same document
// with only that key of that rule changed, or the edit is refused.
func SetRuleField(file string, data []byte, i int, key, value string) ([]byte, error) {
	var before map[string]any
	if _, err := toml.Decode(string(data), &before); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	text := string(data)
	noNL := !strings.HasSuffix(text, "\n")
	if noNL {
		text += "\n"
	}
	lines := strings.SplitAfter(text, "\n")
	cores := ruleCores(lines)
	if i < 0 || i >= len(cores) {
		return nil, fmt.Errorf("%s: no rule #%d", file, i+1)
	}
	// ruleCores starts a core at the comment lines above the header.
	head := cores[i].lo
	for head < len(lines) && !ruleHeader.MatchString(strings.TrimRight(lines[head], "\r\n")) {
		head++
	}
	// The rule's keys run to the next table header; a blank line or a comment
	// can sit between them.
	end := head + 1
	for end < len(lines) && !tableHeader.MatchString(lines[end]) {
		end++
	}

	changed := false
	for n := head + 1; n < end; n++ {
		m := keyLine.FindStringSubmatch(lines[n])
		if m == nil || m[1] != key {
			continue
		}
		changed = true
		if value == "" {
			lines = append(lines[:n], lines[n+1:]...)
			break
		}
		nl := lineEnding(lines[n])
		if m := simpleValue.FindStringSubmatch(strings.TrimRight(lines[n], "\r\n")); m != nil {
			lines[n] = m[1] + value + m[3] + nl
		} else {
			lines[n] = key + " = " + value + nl
		}
		break
	}
	if !changed {
		if value == "" {
			return data, nil
		}
		line := key + " = " + value + lineEnding(lines[head])
		lines = append(lines[:head+1], append([]string{line}, lines[head+1:]...)...)
	}
	res := strings.Join(lines, "")
	if noNL {
		res = strings.TrimSuffix(res, "\n")
	}

	var after map[string]any
	_, err := toml.Decode(res, &after)
	if rs, ok := before["rule"].([]map[string]any); ok && err == nil && i < len(rs) {
		if value == "" {
			delete(rs[i], key)
		} else {
			var v map[string]any
			if _, err := toml.Decode("v = "+value, &v); err == nil {
				rs[i][key] = v["v"]
			}
		}
		if reflect.DeepEqual(before, after) {
			return []byte(res), nil
		}
	}
	return nil, fmt.Errorf("%s: can't set %s on rule #%d safely; edit the file (e)", file, key, i+1)
}

// lineEnding is the line terminator of l: "\r\n" or "\n" (the last line of a
// file without a final newline takes "\n").
func lineEnding(l string) string {
	if strings.HasSuffix(l, "\r\n") {
		return "\r\n"
	}
	return "\n"
}
