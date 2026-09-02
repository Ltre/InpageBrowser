package server

import (
	"net/http/httptest"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"example.com":           "https://example.com",
		"family research":       "https://www.google.com/search?q=family+research",
		"https://example.com/a": "https://example.com/a",
	}
	for in, want := range cases {
		got, err := normalizeURL(in)
		if err != nil || got != want {
			t.Fatalf("%q => %q %v, want %q", in, got, err, want)
		}
	}
}

func TestOriginBehindCloudflare(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.test/", nil)
	r.Host = "browser.example.com"
	r.Header.Set("X-Forwarded-Proto", "https")
	origin, rp := requestOriginRP(r)
	if origin != "https://browser.example.com" || rp != "browser.example.com" {
		t.Fatalf("%s %s", origin, rp)
	}
}
