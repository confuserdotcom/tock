package classify

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

// InsertRuleText is data, the text of one rule file, with r added as its
// first rule (before the comment lines above the old first rule), so it is
// checked before the file's other rules. A file without rules gets it at
// the end. Text that does not parse is an error.
func InsertRuleText(file string, data []byte, r Rule, comment string) ([]byte, error) {
	var doc map[string]any
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return nil, fmt.Errorf("%s: %w (not modified)", file, err)
	}
	block := renderRule(r, comment)
	text := string(data)
	lines := strings.SplitAfter(text, "\n")
	cores := ruleCores(lines)
	if len(cores) == 0 {
		if strings.TrimSpace(text) == "" {
			return []byte(block), nil
		}
		return []byte(strings.TrimRight(text, "\n") + "\n\n" + block), nil
	}
	at := 0
	for _, l := range lines[:cores[0].lo] {
		at += len(l)
	}
	return []byte(text[:at] + block + text[at:]), nil
}
