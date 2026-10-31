package classify

import (
	"strings"
	"testing"
)

func TestMeetingLink(t *testing.T) {
	long := "https://zoom.us/j/" + strings.Repeat("1", 2049-len("https://zoom.us/j/"))
	if len(long) != 2049 {
		t.Fatalf("fixture length %d", len(long))
	}
	cases := []struct{ in, want string }{
		{"https://meet.google.com/abc-defg-hij", "https://meet.google.com/abc-defg-hij"},
		{"Join: https://us02web.zoom.us/j/123?pwd=x.", "https://us02web.zoom.us/j/123?pwd=x"},
		{"<https://teams.microsoft.com/l/meetup-join/19%3ameeting_x%40thread.v2/0?context=%7b%7d>", "https://teams.microsoft.com/l/meetup-join/19%3ameeting_x%40thread.v2/0?context=%7b%7d"},
		{"https://teams.live.com/meet/9", "https://teams.live.com/meet/9"},
		{"https://zoom.us/j/1", "https://zoom.us/j/1"},
		{"(see https://zoom.us/j/1)", "https://zoom.us/j/1"},
		{"http://zoom.us/j/1", ""},
		{"https://evilzoom.us/j/1", ""},
		{"https://zoom.us.evil.com/j/1", ""},
		{"https://meet.google.com@evil.com/x", ""},
		{"https://u:p@meet.google.com/x", ""},
		{"https://meet.google.com:8443/x", ""},
		{"javascript:alert(1)", ""},
		{"file:///etc/passwd", ""},
		{"", ""},
		{"https://www.google.com/url?q=https://meet.google.com/abc-defg-hij", "https://meet.google.com/abc-defg-hij"},
		{long, ""},
	}
	for _, c := range cases {
		if got := MeetingLink(c.in); got != c.want {
			n := len(c.in)
			if n > 60 {
				n = 60
			}
			t.Errorf("MeetingLink(%q) = %q, want %q", c.in[:n], got, c.want)
		}
	}
	if got := MeetingLink("", "https://zoom.us/j/1", "https://meet.google.com/x"); got != "https://zoom.us/j/1" {
		t.Errorf("first field = %q", got)
	}
	if got := MeetingLink("https://evil.example/x", "no link", "https://meet.google.com/x"); got != "https://meet.google.com/x" {
		t.Errorf("skip bad field = %q", got)
	}
	if got := MeetingLink(); got != "" {
		t.Errorf("no args = %q", got)
	}
}

func TestIsMeetingLink(t *testing.T) {
	for s, want := range map[string]bool{
		"https://meet.google.com/x":     true,
		"https://us02web.zoom.us/j/1":   true,
		" https://meet.google.com/x":    false,
		"https://meet.google.com/x y":   false,
		"https://meet.google.com/x\n":   false,
		"http://meet.google.com/x":      false,
		"https://evil.example/x":        false,
		"https://zoom.us.evil.com/x":    false,
		"https://u@zoom.us/x":           false,
		"https://zoom.us:444/x":         false,
		"javascript:x":                  false,
		"":                              false,
		"https://meet.google.com/\x01x": false,
	} {
		if got := IsMeetingLink(s); got != want {
			t.Errorf("IsMeetingLink(%q) = %v, want %v", s, got, want)
		}
	}
}
