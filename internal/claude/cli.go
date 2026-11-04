package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// CLITimeout bounds one call through the claude CLI, which starts a
// process before it reaches the API.
const CLITimeout = 120 * time.Second

// Runner runs path with args and stdin and returns its stdout.
type Runner func(ctx context.Context, path string, args []string, stdin []byte) ([]byte, error)

// CLI calls Claude through the official claude CLI in print mode, on the
// user's Claude subscription: no API key. The payload goes on stdin, with
// no tools, MCP servers, slash commands, settings or saved session.
type CLI struct {
	Path string
	Run  Runner
}

// NewCLI returns a CLI that runs the claude binary at path.
func NewCLI(path string) *CLI { return &CLI{Path: path, Run: execRun} }

// FindCLI returns the claude binary on PATH, else ~/.local/bin/claude, else "".
func FindCLI() string {
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := home + "/.local/bin/claude"
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

func execRun(ctx context.Context, path string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = os.TempDir() // no project CLAUDE.md
	cmd.Stdin = bytes.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.Bytes(), err
}

// Suggest is Client.Suggest through the CLI.
func (c *CLI) Suggest(ctx context.Context, model string, payload []byte) (json.RawMessage, error) {
	return c.call(ctx, model, system, tool, payload)
}

// Summarize is Client.Summarize through the CLI.
func (c *CLI) Summarize(ctx context.Context, model string, payload []byte) (json.RawMessage, error) {
	return c.call(ctx, model, summarySystem, summaryTool, payload)
}

// Ask is Client.Ask through the CLI.
func (c *CLI) Ask(ctx context.Context, model string, payload []byte) (json.RawMessage, error) {
	return c.call(ctx, model, askSystem, askTool, payload)
}

// call asks for structured output shaped like t's input schema and returns
// it, as the API path returns the tool call's input. Errors never carry the
// payload or the CLI's output.
func (c *CLI) call(ctx context.Context, model, sys string, t json.RawMessage, payload []byte) (json.RawMessage, error) {
	var spec struct {
		Schema json.RawMessage `json:"input_schema"`
	}
	if err := json.Unmarshal(t, &spec); err != nil || len(spec.Schema) == 0 {
		return nil, errors.New("claude: bad tool schema")
	}
	args := []string{"-p", "--output-format", "json", "--model", model,
		"--system-prompt", sys, "--json-schema", string(spec.Schema),
		"--tools", "", "--strict-mcp-config", "--disable-slash-commands",
		"--no-session-persistence", "--setting-sources", ""}
	out, err := c.Run(ctx, c.Path, args, payload)
	if ctx.Err() != nil {
		return nil, fmt.Errorf("claude CLI: %w", ctx.Err())
	}
	var r struct {
		IsError bool            `json:"is_error"`
		Subtype string          `json:"subtype"`
		Output  json.RawMessage `json:"structured_output"`
	}
	if jerr := json.Unmarshal(out, &r); jerr != nil {
		if err != nil {
			return nil, fmt.Errorf("claude CLI failed: %w (is it logged in? run claude once)", err)
		}
		return nil, errors.New("claude CLI: output is not JSON")
	}
	if r.IsError || err != nil {
		sub := r.Subtype
		if !safeType.MatchString(sub) {
			sub = "unknown_error"
		}
		return nil, fmt.Errorf("claude CLI: %s", sub)
	}
	if len(r.Output) == 0 || string(r.Output) == "null" {
		return nil, errors.New("claude CLI: no structured output")
	}
	return r.Output, nil
}
