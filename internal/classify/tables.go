package classify

import (
	"fmt"
	"regexp"
	"strings"
)

// Field is one `key = value` line of a [[category]] or [[project]] table.
// Value is TOML text: TOMLString renders a string.
type Field struct{ Key, Value string }

// TOMLString renders s as a TOML basic string.
func TOMLString(s string) string { return tomlString(s) }

// TOMLStrings renders ss as a TOML array of strings.
func TOMLStrings(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = tomlString(s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

var (
	keyLine = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_-]*)\s*=`)
	// simpleValue is a key line whose value is one string or bare token,
	// so its spacing and trailing comment can be kept.
	simpleValue = regexp.MustCompile(`^(\s*[A-Za-z_][A-Za-z0-9_-]*\s*=\s*)("(?:[^"\\]|\\.)*"|'[^']*'|[^#\s"']+)(\s*(?:#.*)?)$`)
)

// tableSpan finds the [[kind]] table named name: its header line and the
// line after its last key line. ok is false when there is none.
func tableSpan(lines []string, kind, name string) (lo, hi int, ok bool) {
	for n, l := range lines {
		m := tableStart.FindStringSubmatch(strings.TrimRight(l, "\r\n"))
		if m == nil || m[1] != kind {
			continue
		}
		end := n + 1
		match := false
		for end < len(lines) && !isBlank(lines[end]) && !tableHeader.MatchString(lines[end]) {
			if nm := nameLine.FindStringSubmatch(lines[end]); nm != nil && strings.EqualFold(strings.TrimSpace(unquote(nm[1])), strings.TrimSpace(name)) {
				match = true
			}
			end++
		}
		if match {
			return n, end, true
		}
	}
	return 0, 0, false
}

// AddTable appends a [[kind]] table with fields after the last table of that
// kind; with none, before the first [[project]], [[rule]] or [[goal]] table,
// else at the end.
func AddTable(text, kind string, fields []Field) string {
	var b strings.Builder
	b.WriteString("[[" + kind + "]]\n")
	for _, f := range fields {
		b.WriteString(f.Key + " = " + f.Value + "\n")
	}
	table := b.String()
	lines := strings.SplitAfter(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	at := -1
	for n, l := range lines {
		m := tableStart.FindStringSubmatch(strings.TrimRight(l, "\r\n"))
		if m == nil || m[1] != kind {
			continue
		}
		end := n + 1
		for end < len(lines) && !isBlank(lines[end]) && !tableHeader.MatchString(lines[end]) {
			end++
		}
		at = end
	}
	if at < 0 {
		for n, l := range lines {
			h := strings.TrimSpace(l)
			if h == "[[project]]" || h == "[[rule]]" || h == "[[goal]]" {
				at = n
				for at > 0 && isComment(lines[at-1]) {
					at--
				}
				return strings.Join(lines[:at], "") + table + "\n" + strings.Join(lines[at:], "")
			}
		}
		head := strings.Join(lines, "")
		if head != "" && !strings.HasSuffix(head, "\n") {
			head += "\n"
		}
		if head != "" && !strings.HasSuffix(head, "\n\n") {
			head += "\n"
		}
		return head + table
	}
	return strings.Join(lines[:at], "") + "\n" + table + strings.Join(lines[at:], "")
}

// SetTableField sets key in the [[kind]] table named name to value (TOML
// text), replacing its line or adding one after the name; value "" removes
// the line.
func SetTableField(text, kind, name, key, value string) (string, error) {
	lines := strings.SplitAfter(text, "\n")
	lo, hi, ok := tableSpan(lines, kind, name)
	if !ok {
		return "", fmt.Errorf("no [[%s]] named %q in %s", kind, name, RulesFile)
	}
	nameAt := lo
	for i := lo + 1; i < hi; i++ {
		m := keyLine.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		if m[1] == "name" {
			nameAt = i
		}
		if m[1] != key {
			continue
		}
		if value == "" {
			return strings.Join(lines[:i], "") + strings.Join(lines[i+1:], ""), nil
		}
		nl := ""
		if strings.HasSuffix(lines[i], "\n") {
			nl = "\n"
		}
		if m := simpleValue.FindStringSubmatch(strings.TrimRight(lines[i], "\r\n")); m != nil {
			lines[i] = m[1] + value + m[3] + nl
		} else {
			lines[i] = key + " = " + value + nl
		}
		return strings.Join(lines, ""), nil
	}
	if value == "" {
		return text, nil
	}
	line := key + " = " + value + "\n"
	if !strings.HasSuffix(lines[nameAt], "\n") {
		lines[nameAt] += "\n"
	}
	return strings.Join(lines[:nameAt+1], "") + line + strings.Join(lines[nameAt+1:], ""), nil
}

// RenameName renames the declared category or project from to the new name
// to, which must not be declared yet: its table's name and every
// `kind = "from"` value in both files. The result must parse with the same
// number of names and rules.
func RenameName(rulesData, learnedData []byte, kind, from, to string) (Merged, error) {
	rs, err := Parse(rulesData, learnedData)
	if err != nil {
		return Merged{}, err
	}
	to = strings.TrimSpace(to)
	byLower := func(n string) (string, bool) {
		if kind == NameCategory {
			c, ok := rs.catByLower[nameKey(n)]
			return c.Name, ok
		}
		p, ok := rs.projByLower[nameKey(n)]
		return p.Name, ok
	}
	src, ok := byLower(from)
	if !ok {
		return Merged{}, fmt.Errorf("rename: no %s %q", kind, from)
	}
	switch other, taken := byLower(to); {
	case to == "" || strings.Contains(to, "/"):
		return Merged{}, fmt.Errorf("rename: %q is not a name (empty or with /)", to)
	case taken && !strings.EqualFold(other, src):
		return Merged{}, fmt.Errorf("rename: %s %q exists: merge into it instead (tock rules merge)", kind, other)
	case to == src:
		return Merged{}, fmt.Errorf("rename: %q is the same name", src)
	}
	rules, err := SetTableField(renameValues(string(rulesData), kind, src, to), kind, src, "name", tomlString(to))
	if err != nil {
		return Merged{}, err
	}
	learned := []byte(renameValues(string(learnedData), kind, src, to))
	after, err := Parse([]byte(rules), learned)
	if err != nil {
		return Merged{}, fmt.Errorf("rename: the rewritten files do not parse (%v); edit them by hand", err)
	}
	if len(after.Categories) != len(rs.Categories) || len(after.Projects) != len(rs.Projects) ||
		len(after.Rules) != len(rs.Rules) || len(after.Learned) != len(rs.Learned) {
		return Merged{}, fmt.Errorf("rename: can't rewrite this layout safely; edit rules.toml by hand")
	}
	return Merged{From: src, To: to, Rules: []byte(rules), Learned: learned}, nil
}

// Uses counts what names a category or project: rules in both files,
// goals, the [calls] and [calendar] tables, and (for a category) the
// projects filed under it.
func (rs *Ruleset) Uses(kind, name string) int {
	n := 0
	eq := func(s string) bool { return s != "" && strings.EqualFold(s, name) }
	for _, rules := range [][]Rule{rs.Learned, rs.Rules} {
		for _, r := range rules {
			if (kind == NameCategory && eq(r.Category)) || (kind == NameProject && eq(r.Project)) {
				n++
			}
		}
	}
	for _, g := range rs.Goals {
		if (kind == NameCategory && eq(g.Category)) || (kind == NameProject && eq(g.Project)) {
			n++
		}
	}
	if kind == NameCategory {
		if eq(rs.CallCategory) {
			n++
		}
		if eq(rs.CalendarCategory) {
			n++
		}
		for _, p := range rs.Projects {
			if eq(p.Category) {
				n++
			}
		}
	}
	return n
}

// RemoveName drops the [[kind]] table named name from rules.toml. Nothing
// may name it any more (Uses is 0): merge it into another name instead.
func RemoveName(rulesData, learnedData []byte, kind, name string) ([]byte, string, error) {
	rs, err := Parse(rulesData, learnedData)
	if err != nil {
		return nil, "", err
	}
	var decl string
	if kind == NameCategory {
		decl = rs.catByLower[nameKey(name)].Name
	} else {
		decl = rs.projByLower[nameKey(name)].Name
	}
	if decl == "" {
		return nil, "", fmt.Errorf("remove: no %s %q", kind, name)
	}
	if n := rs.Uses(kind, decl); n > 0 {
		return nil, "", fmt.Errorf("%s is named by %s: merge it into another %s instead", decl, plural(n, "rule or goal", "rules or goals"), kind)
	}
	out := []byte(dropTable(string(rulesData), kind, decl))
	if _, err := Parse(out, learnedData); err != nil {
		return nil, "", fmt.Errorf("remove: the rewritten file does not parse (%v); edit it by hand", err)
	}
	return out, decl, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
