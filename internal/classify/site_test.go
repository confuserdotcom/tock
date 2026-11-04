package classify

import "testing"

func TestSiteOf(t *testing.T) {
	for _, tc := range []struct{ url, site, domain string }{
		{"chatgpt.com/c/1", "chatgpt.com", ""},
		{"www.youtube.com/watch?v=x", "youtube.com", ""},
		{"docs.google.com/document/d/1", "docs.google.com", ""},
		{"app.noctie.ai/opening-book", "app.noctie.ai", ""},
		{"doc-0o-bs-apps-viewer.googleusercontent.com/viewer/secure/pdf/x", "googleusercontent.com", "googleusercontent.com"},
		{"instagram.ffez2-2.fna.fbcdn.net/v/t51", "fbcdn.net", "fbcdn.net"},
		{"a1.example.co.uk/x", "example.co.uk", "example.co.uk"},
		{"cdn7.site.com.ma", "site.com.ma", "site.com.ma"},
		{"localhost:3000/index.html", "localhost:3000", ""},
		{"192.168.1.10/admin", "192.168.1.10", ""},
		{"2x.com/a", "2x.com", ""},
	} {
		site, domain := siteOf(tc.url)
		if site != tc.site || domain != tc.domain {
			t.Errorf("siteOf(%q) = %q, %q; want %q, %q", tc.url, site, domain, tc.site, tc.domain)
		}
	}
}

// TestKeyIsSite: every page of a site shares one inbox key.
func TestKeyIsSite(t *testing.T) {
	a := SpanView{App: "Arc", URL: sp("www.youtube.com/watch?v=1")}
	b := SpanView{App: "Arc", URL: sp("youtube.com/@channel/videos")}
	c := SpanView{App: "Arc", URL: sp("doc-0o-bs.googleusercontent.com/viewer/1")}
	d := SpanView{App: "Arc", URL: sp("doc-1k-aa.googleusercontent.com/viewer/2")}
	if Key(a) != "Arc · youtube.com" || Key(b) != Key(a) {
		t.Fatalf("youtube keys = %q, %q", Key(a), Key(b))
	}
	if Key(c) != "Arc · googleusercontent.com" || Key(d) != Key(c) {
		t.Fatalf("cdn keys = %q, %q", Key(c), Key(d))
	}
	if LegacyKey(a) != "Arc · youtube.com/watch?v=1" && LegacyKey(a) != "Arc · youtube.com/watch" {
		t.Fatalf("legacy key = %q", LegacyKey(a))
	}
}

// TestGroupCountsPages: a site row says how many distinct pages it holds.
func TestGroupCountsPages(t *testing.T) {
	views := []SpanView{
		{Start: 0, End: 10, App: "Arc", URL: sp("youtube.com/watch?v=1")},
		{Start: 10, End: 20, App: "Arc", URL: sp("youtube.com/watch?v=2")},
		{Start: 20, End: 30, App: "Arc", URL: sp("youtube.com/@chan")},
	}
	items := Group(views, func(int) bool { return true })
	if len(items) != 1 || items[0].Key != "Arc · youtube.com" || items[0].Pages != 2 {
		t.Fatalf("items = %+v, want one youtube.com row with 2 pages", items)
	}
}

// TestInboxHonoursLegacyIgnore: a key ignored before site grouping stays hidden.
func TestInboxHonoursLegacyIgnore(t *testing.T) {
	rs := mustParse(t, labelRules, "")
	views := []SpanView{{ID: 1, Start: 0, End: 60_000, BundleID: "arc", App: "Arc", URL: sp("chatgpt.com/c/1")}}
	items := Inbox(views, rs.LabelAll(views, nil), map[string]bool{"Arc · chatgpt.com/c": true})
	if len(items) != 0 {
		t.Fatalf("items = %+v, want the legacy-ignored key hidden", items)
	}
}
