package orderx

import "testing"

func TestCompactParseAcceptsOrderURL(t *testing.T) {
	data := []byte(`{"orders":[{"id":"9","title":"PROJ-123 fix login bug","url":"https://jira.example.com/browse/PROJ-123","stages":[{"do":"execute"}]}]}`)

	compact, err := ParseCompactOrders(data)
	if err != nil {
		t.Fatalf("ParseCompactOrders returned error: %v", err)
	}
	expanded, err := ExpandCompactOrders(compact)
	if err != nil {
		t.Fatalf("ExpandCompactOrders returned error: %v", err)
	}
	if got := expanded.Orders[0].URL; got != "https://jira.example.com/browse/PROJ-123" {
		t.Fatalf("URL = %q", got)
	}
}

func TestCleanOrderURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/o/r/pull/12":  "https://github.com/o/r/pull/12",
		"  http://example.com/x  ":        "http://example.com/x",
		"":                                "",
		"PROJ-123":                        "",
		"jira.example.com/browse/PROJ-1":  "",
		"javascript:alert(1)":             "",
		"ftp://example.com/file":          "",
		"https://":                        "",
		"https://exa mple.com/with space": "",
	}
	for in, want := range cases {
		if got := CleanOrderURL(in); got != want {
			t.Errorf("CleanOrderURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCompactExpandDropsMalformedOrderURL(t *testing.T) {
	compact := CompactOrdersFile{Orders: []CompactOrder{{
		ID:     "1",
		Title:  "t",
		URL:    "javascript:alert(1)",
		Stages: []CompactStage{{Do: "execute"}},
	}}}
	expanded, err := ExpandCompactOrders(compact)
	if err != nil {
		t.Fatalf("ExpandCompactOrders returned error: %v", err)
	}
	if got := expanded.Orders[0].URL; got != "" {
		t.Fatalf("URL = %q, want empty", got)
	}
}
