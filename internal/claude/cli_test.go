package claude

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestCLISuggest(t *testing.T) {
	var gotArgs []string
	var gotIn []byte
	c := &CLI{Path: "/x/claude", Run: func(_ context.Context, path string, args []string, stdin []byte) ([]byte, error) {
		if path != "/x/claude" {
			t.Errorf("path = %q", path)
		}
		gotArgs, gotIn = args, stdin
		return []byte(`{"type":"result","subtype":"success","is_error":false,"result":"…","structured_output":{"suggestions":[{"id":"k1","category":"Study","project":null}]}}`), nil
	}}
	raw, err := c.Suggest(context.Background(), "claude-haiku-5-5", []byte(`{"items":[]}`))
	if err != nil || !strings.Contains(string(raw), `"k1"`) {
		t.Fatalf("Suggest = %s, %v", raw, err)
	}
	if string(gotIn) != `{"items":[]}` {
		t.Fatalf("stdin = %s", gotIn)
	}
	i := slices.Index(gotArgs, "--json-schema")
	if i < 0 || !json.Valid([]byte(gotArgs[i+1])) || !strings.Contains(gotArgs[i+1], `"suggestions"`) {
		t.Fatalf("schema arg: %v", gotArgs)
	}
	for _, want := range []string{"-p", "--no-session-persistence", "--strict-mcp-config", "--disable-slash-commands"} {
		if !slices.Contains(gotArgs, want) {
			t.Errorf("args lack %s", want)
		}
	}
	if j := slices.Index(gotArgs, "--tools"); j < 0 || gotArgs[j+1] != "" {
		t.Fatalf("tools must be disabled: %v", gotArgs)
	}
	if j := slices.Index(gotArgs, "--model"); j < 0 || gotArgs[j+1] != "claude-haiku-5-5" {
		t.Fatalf("model: %v", gotArgs)
	}
}

func TestCLIErrorsHideOutput(t *testing.T) {
	for _, tc := range []struct {
		out  string
		err  error
		want string
	}{
		{`{"is_error":true,"subtype":"error_during_execution","result":"SECRET"}`, nil, "claude CLI: error_during_execution"},
		{`not json SECRET`, errors.New("exit status 1"), "is it logged in?"},
		{`not json SECRET`, nil, "output is not JSON"},
		{`{"is_error":false,"subtype":"success","result":"SECRET"}`, nil, "no structured output"},
		{`{"is_error":true,"subtype":"SECRET stuff"}`, nil, "unknown_error"},
	} {
		c := &CLI{Run: func(context.Context, string, []string, []byte) ([]byte, error) { return []byte(tc.out), tc.err }}
		_, err := c.Ask(context.Background(), "m", []byte("payload SECRET"))
		if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "SECRET") {
			t.Errorf("%s: err = %v, want %q", tc.out, err, tc.want)
		}
	}
}
