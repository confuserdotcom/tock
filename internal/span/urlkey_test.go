package span

import "testing"

func TestURLKeyStripsQueryFragmentUserinfo(t *testing.T) {
	got, ok := URLKey("https://user:pw@Example.COM:8080/a/b/?q=1#f")
	if !ok || got != "example.com:8080/a/b" {
		t.Fatalf("URLKey = %q, %v; want %q, true", got, ok, "example.com:8080/a/b")
	}
}

func TestURLKeyRootIsHostOnly(t *testing.T) {
	for _, raw := range []string{"https://example.com", "https://example.com/", "http://EXAMPLE.com/?x=1"} {
		got, ok := URLKey(raw)
		if !ok || got != "example.com" {
			t.Errorf("URLKey(%q) = %q, %v; want %q, true", raw, got, ok, "example.com")
		}
	}
}

func TestURLKeyNonHTTPRejected(t *testing.T) {
	for _, raw := range []string{
		"file:///etc/passwd",
		"about:blank",
		"chrome://settings",
		"data:text/html,hi",
		"",
	} {
		if got, ok := URLKey(raw); ok {
			t.Errorf("URLKey(%q) = %q, true; want rejected", raw, got)
		}
	}
}

func TestURLKeyGarbageRejected(t *testing.T) {
	long := "https://example.com/" + string(make([]byte, maxURLLen))
	for _, raw := range []string{
		"http://[::1",
		"https://",
		"https:///path-only",
		"not a url",
		"%zz",
		long,
	} {
		if got, ok := URLKey(raw); ok {
			t.Errorf("URLKey(%q) = %q, true; want rejected", raw, got)
		}
	}
}
