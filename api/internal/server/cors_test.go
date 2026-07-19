package server

import "testing"

func TestOriginAllowed(t *testing.T) {
	pinned := map[string]bool{"https://myfipay.com": true, "https://www.myfipay.com": true}
	cases := []struct {
		name    string
		origin  string
		allowed map[string]bool
		dev     bool
		want    bool
	}{
		{"pinned origin", "https://myfipay.com", pinned, false, true},
		{"pinned origin trailing slash", "https://myfipay.com/", pinned, false, true},
		{"unpinned origin prod", "https://evil.example", pinned, false, false},
		{"unpinned origin dev with pins still blocked", "https://evil.example", pinned, true, false},
		{"empty origin", "", pinned, true, false},
		{"no pins dev echoes", "http://localhost:3000", map[string]bool{}, true, true},
		{"no pins prod blocks", "https://anything.example", map[string]bool{}, false, false},
	}
	for _, c := range cases {
		if got := originAllowed(c.origin, c.allowed, c.dev); got != c.want {
			t.Errorf("%s: originAllowed(%q, dev=%v) = %v, want %v", c.name, c.origin, c.dev, got, c.want)
		}
	}
}
