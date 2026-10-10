package suggest

import (
	"testing"

	"tock/internal/classify"
	"tock/internal/config"
)

const minute = 60_000

const testRules = `
[[category]]
name = "Code"
kind = "deep"

[[category]]
name = "Video"
kind = "distract"

[[project]]
name = "tock"

[[project]]
name = "Old"
archived = true

[[rule]]
bundle = ["term"]
category = "Code"
`

func ruleset(t *testing.T) *classify.Ruleset {
	t.Helper()
	rs, err := classify.Parse([]byte(testRules), nil)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func filter(t *testing.T, c config.LLM) *Filter {
	t.Helper()
	f, err := NewFilter(c)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
