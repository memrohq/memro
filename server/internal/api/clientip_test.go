package api

import (
	"net/http/httptest"
	"testing"

	"github.com/memrohq/memro/server/config"
)

func TestClientIPIgnoresForgedForwardedFor(t *testing.T) {
	s := &Server{cfg: config.Config{TrustProxyHeaders: true}}
	cases := []struct {
		name, realIP, fwd, want string
	}{
		{"proxy sets X-Real-IP", "203.0.113.7", "6.6.6.6, 203.0.113.7", "203.0.113.7"},
		{"forged first X-Forwarded-For entry", "", "6.6.6.6, 203.0.113.7", "203.0.113.7"},
		{"single X-Forwarded-For entry", "", "203.0.113.7", "203.0.113.7"},
		{"no proxy headers", "", "", "192.0.2.1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "192.0.2.1:1234"
		if c.realIP != "" {
			r.Header.Set("X-Real-IP", c.realIP)
		}
		if c.fwd != "" {
			r.Header.Set("X-Forwarded-For", c.fwd)
		}
		if got := s.clientIP(r); got != c.want {
			t.Errorf("%s: clientIP = %q, want %q", c.name, got, c.want)
		}
	}

	untrusted := &Server{cfg: config.Config{}}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("X-Real-IP", "6.6.6.6")
	if got := untrusted.clientIP(r); got != "192.0.2.1" {
		t.Errorf("untrusted proxy headers were used: %q", got)
	}
}
