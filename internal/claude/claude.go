// Package claude is tock's only network code: one call to the Anthropic
// Messages API that asks for inbox label suggestions. Only cmd/tock imports
// it; trackd never links it (cmd/trackd tests check). Errors carry the HTTP
// status and API error type only, never the request, the body or the key.
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

const (
	// DefaultURL is the Messages API endpoint.
	DefaultURL = "https://api.anthropic.com/v1/messages"
	// Timeout bounds one request.
	Timeout = 30 * time.Second

	apiVersion = "2023-06-01"
	maxTokens  = 4096
)

const system = `You label activity for tock, a personal time tracker. Each item is one app with either a URL (host and first path segment) or a normalised window title (lowercase, digits replaced by #), and the minutes spent on it. Categories have a kind: deep (focused work or study), shallow (necessary light work), distract (entertainment, social media, idle browsing), neutral (everything else). For every item choose the best category and, only when one clearly fits, a project, using only the exact names given. Use null when unsure. Call the suggest tool once with an entry for every item id.`

var tool = json.RawMessage(`{
  "name": "suggest",
  "description": "Suggested category and project for each inbox item.",
  "input_schema": {
    "type": "object",
    "properties": {
      "suggestions": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "id": {"type": "string"},
            "category": {"type": ["string", "null"]},
            "project": {"type": ["string", "null"]}
          },
          "required": ["id", "category", "project"]
        }
      }
    },
    "required": ["suggestions"]
  }
}`)

// Client calls the Messages API with one key.
type Client struct {
	Key  string
	URL  string
	HTTP *http.Client
}

// New returns a client for the public endpoint.
func New(key string) *Client {
	return &Client{Key: key, URL: DefaultURL, HTTP: &http.Client{Timeout: Timeout}}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type toolChoice struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type request struct {
	Model      string            `json:"model"`
	MaxTokens  int               `json:"max_tokens"`
	System     string            `json:"system"`
	Messages   []message         `json:"messages"`
	Tools      []json.RawMessage `json:"tools"`
	ToolChoice toolChoice        `json:"tool_choice"`
}

var safeType = regexp.MustCompile(`^[a-z_]{1,40}$`)

// Suggest sends payload as the user message and returns the input of the
// suggest tool call.
func (c *Client) Suggest(ctx context.Context, model string, payload []byte) (json.RawMessage, error) {
	body, err := json.Marshal(request{
		Model: model, MaxTokens: maxTokens, System: system,
		Messages:   []message{{Role: "user", Content: string(payload)}},
		Tools:      []json.RawMessage{tool},
		ToolChoice: toolChoice{Type: "tool", Name: "suggest"},
	})
	if err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	req.Header.Set("x-api-key", c.Key)
	req.Header.Set("anthropic-version", apiVersion)
	req.Header.Set("content-type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, errors.New("claude: cannot read the response")
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error struct {
				Type string `json:"type"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		typ := e.Error.Type
		if !safeType.MatchString(typ) {
			typ = "unknown_error"
		}
		return nil, fmt.Errorf("HTTP %d %s", resp.StatusCode, typ)
	}
	var r struct {
		Content []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, errors.New("claude: response is not JSON")
	}
	for _, b := range r.Content {
		if b.Type == "tool_use" && b.Name == "suggest" {
			return b.Input, nil
		}
	}
	return nil, errors.New("claude: response has no suggest tool call")
}
