package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseIPList(t *testing.T) {
	nets, err := ParseIPList("188.245.65.108, 10.0.0.0/8,, 2001:db8::1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(nets) != 3 {
		t.Fatalf("want 3 nets, got %d", len(nets))
	}
	if _, err := ParseIPList("not-an-ip"); err == nil {
		t.Fatal("want error for garbage input")
	}
	if _, err := ParseIPList("10.0.0.0/99"); err == nil {
		t.Fatal("want error for bad CIDR")
	}
	empty, err := ParseIPList("")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty list should parse to no nets, got %v %v", empty, err)
	}
}

func TestIPAllowed(t *testing.T) {
	nets, _ := ParseIPList("188.245.65.108,10.0.0.0/8")
	cases := []struct {
		ip   string
		want bool
	}{
		{"188.245.65.108", true},
		{"188.245.65.109", false},
		{"10.1.2.3", true},
		{"11.1.2.3", false},
		{"garbage", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IPAllowed(c.ip, nets); got != c.want {
			t.Errorf("IPAllowed(%q) = %v, want %v", c.ip, got, c.want)
		}
	}
}

func TestClientIPPrefersXRealIP(t *testing.T) {
	r := httptest.NewRequest("POST", "/webhooks/zengapay", nil)
	r.RemoteAddr = "127.0.0.1:9999"
	r.Header.Set("X-Real-IP", "188.245.65.108")
	// forged headers that must NOT win
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("True-Client-IP", "1.2.3.4")
	if got := ClientIP(r); got != "188.245.65.108" {
		t.Fatalf("ClientIP = %q, want X-Real-IP value", got)
	}

	r2 := httptest.NewRequest("POST", "/", nil)
	r2.RemoteAddr = "127.0.0.1:9999"
	if got := ClientIP(r2); got != "127.0.0.1" {
		t.Fatalf("ClientIP fallback = %q, want 127.0.0.1", got)
	}
}

func TestRequireSourceIP(t *testing.T) {
	nets, _ := ParseIPList("188.245.65.108")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })

	send := func(realIP string, nets []*net.IPNet) int {
		r := httptest.NewRequest("POST", "/webhooks/zengapay", nil)
		r.RemoteAddr = "203.0.113.7:1234"
		if realIP != "" {
			r.Header.Set("X-Real-IP", realIP)
		}
		w := httptest.NewRecorder()
		RequireSourceIP(nets)(next).ServeHTTP(w, r)
		return w.Code
	}

	if code := send("188.245.65.108", nets); code != 200 {
		t.Errorf("allowed IP: got %d, want 200", code)
	}
	if code := send("9.9.9.9", nets); code != 403 {
		t.Errorf("blocked IP: got %d, want 403", code)
	}
	if code := send("9.9.9.9", nil); code != 200 {
		t.Errorf("empty allowlist must disable the check: got %d, want 200", code)
	}
}
