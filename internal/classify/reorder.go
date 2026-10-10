package classify

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// ErrRuleEdge means the rule cannot move further in that direction.
var ErrRuleEdge = errors.New("rule is already at the edge")

var (
	ruleHeader  = regexp.MustCompile(`^\s*\[\[\s*rule\s*\]\]\s*(#.*)?$`)
	tableHeader = regexp.MustCompile(`^\s*\[`)
)

func isComment(l string) bool { return strings.HasPrefix(strings.TrimSpace(l), "#") }
func isBlank(l string) bool   { return strings.TrimSpace(l) == "" }

// core is a rule's lines [lo, hi): its comment lines directly above, the
// header, and its key lines up to the first blank line, comment or header.
type core struct{ lo, hi int }

// SwapRules swaps rule i and i+1 (0-based) in the text of one rule file,
// keeping every other line in place. The result must decode to the same
// document with the two rules exchanged, or the swap is refused.
func SwapRules(file string, data []byte, i int) ([]byte, error) {
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
	var cores []core
	for n, l := range lines {
		if !ruleHeader.MatchString(strings.TrimRight(l, "\r\n")) {
			continue
		}
		lo := n
		for lo > 0 && isComment(lines[lo-1]) {
			lo--
		}
		hi := n + 1
		for hi < len(lines) && !isBlank(lines[hi]) && !isComment(lines[hi]) && !tableHeader.MatchString(lines[hi]) {
			hi++
		}
		cores = append(cores, core{lo, hi})
	}
	if i < 0 || i+1 >= len(cores) {
		return nil, fmt.Errorf("%s: no rules #%d and #%d", file, i+1, i+2)
	}
	a, b := cores[i], cores[i+1]
	for _, l := range lines[a.hi:b.lo] {
		if tableHeader.MatchString(l) {
			return nil, fmt.Errorf("%s: rules #%d and #%d are separated by another table; edit the file (e)", file, i+1, i+2)
		}
	}
	var out strings.Builder
	for _, part := range [][]string{lines[:a.lo], lines[b.lo:b.hi], lines[a.hi:b.lo], lines[a.lo:a.hi], lines[b.hi:]} {
		for _, l := range part {
			out.WriteString(l)
		}
	}
	res := out.String()
	if noNL {
		res = strings.TrimSuffix(res, "\n")
	}

	var after map[string]any
	_, err := toml.Decode(res, &after)
	if rs, ok := before["rule"].([]map[string]any); ok && err == nil {
		rs[i], rs[i+1] = rs[i+1], rs[i]
		if reflect.DeepEqual(before, after) {
			return []byte(res), nil
		}
	}
	return nil, fmt.Errorf("%s: can't reorder this layout safely; edit the file (e)", file)
}

// MoveRule moves rule i (0-based) of the rule file at path one place up or
// down. The file is rewritten atomically; nothing is written on error.
func MoveRule(path, file string, i int, up bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	j := i
	if up {
		j = i - 1
	}
	var probe map[string]any
	if _, err := toml.Decode(string(data), &probe); err != nil {
		return fmt.Errorf("%s: %w (not modified)", file, err)
	}
	n := 0
	if rs, ok := probe["rule"].([]map[string]any); ok {
		n = len(rs)
	}
	if i < 0 || i >= n {
		return fmt.Errorf("%s: no rule #%d", file, i+1)
	}
	if j < 0 || j+1 >= n {
		return ErrRuleEdge
	}
	out, err := SwapRules(file, data, j)
	if err != nil {
		return err
	}
	if bytes.Equal(out, data) {
		return nil
	}
	return writeAtomic(path, "."+strings.TrimSuffix(filepath.Base(path), ".toml")+"-*.toml", out)
}
