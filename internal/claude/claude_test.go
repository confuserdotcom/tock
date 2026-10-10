package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const payload = `{"items":[{"id":"k1","app":"Arc","host":"github.com/tock","minutes":3}]}`

func serve(t *testing.T, status int, body string, check func(r *http.Request, req map[string]any)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &req); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		}
		if check != nil {
			check(r, req)
		}
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	c := New("test-key")
	c.URL = srv.URL
	return c
}

func TestSuggestRequestAndRecordedResponse(t *testing.T) {
	rec, err := os.ReadFile("testdata/response.json")
	if err != nil {
		t.Fatal(err)
	}
	c := serve(t, 200, string(rec), func(r *http.Request, req map[string]any) {
		if r.Method != http.MethodPost || r.Header.Get("x-api-key") != "test-key" ||
			r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("content-type") != "application/json" {
			t.Errorf("method %s headers %v", r.Method, r.Header)
		}
		if req["model"] != "claude-haiku-5-5" || req["max_tokens"] != float64(4096) {
			t.Errorf("model/max_tokens = %v %v", req["model"], req["max_tokens"])
		}
		tc, _ := req["tool_choice"].(map[string]any)
		if tc["type"] != "tool" || tc["name"] != "suggest" {
			t.Errorf("tool_choice = %v", req["tool_choice"])
		}
		tools, _ := req["tools"].([]any)
		if len(tools) != 1 || tools[0].(map[string]any)["name"] != "suggest" {
			t.Errorf("tools = %v", req["tools"])
		}
		msgs, _ := req["messages"].([]any)
		m0, _ := msgs[0].(map[string]any)
		if len(msgs) != 1 || m0["role"] != "user" || m0["content"] != payload {
			t.Errorf("messages = %v", req["messages"])
		}
	})
	raw, err := c.Suggest(context.Background(), "claude-haiku-5-5", []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	json.Compact(&want, []byte(`{"suggestions": [{"id": "k1", "category": "Code", "project": "tock"}, {"id": "k2", "category": null, "project": null}]}`))
	var got bytes.Buffer
	json.Compact(&got, raw)
	if got.String() != want.String() {
		t.Fatalf("input = %s\nwant %s", got.String(), want.String())
	}
}

func TestSuggestErrorsNeverLeak(t *testing.T) {
	tests := []struct {
		status int
		body   string
		want   string
	}{
		{401, `{"type":"error","error":{"type":"authentication_error","message":"SECRETBODY invalid x-api-key"}}`, "HTTP 401 authentication_error"},
		{500, `<html>SECRETBODY</html>`, "HTTP 500 unknown_error"},
		{529, `{"type":"error","error":{"type":"Overloaded Error!","message":"SECRETBODY"}}`, "HTTP 529 unknown_error"},
		{200, `{"content":[{"type":"text","text":"SECRETBODY"}]}`, "claude: response has no suggest tool call"},
		{200, `SECRETBODY`, "claude: response is not JSON"},
	}
	for _, tt := range tests {
		c := serve(t, tt.status, tt.body, nil)
		_, err := c.Suggest(context.Background(), "m", []byte(payload))
		if err == nil || err.Error() != tt.want {
			t.Errorf("status %d: err = %v, want %q", tt.status, err, tt.want)
			continue
		}
		if strings.Contains(err.Error(), "SECRETBODY") || strings.Contains(err.Error(), "test-key") {
			t.Errorf("error leaks: %v", err)
		}
	}
}
