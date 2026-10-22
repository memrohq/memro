package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memrohq/memro/server/config"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func newTestMetricsServer(t *testing.T, token string, rateLimitMax int) (*Server, http.Handler) {
	t.Helper()
	cfg := config.Config{
		MetricsToken:          token,
		RateLimitMax:          rateLimitMax,
		RateLimitWindow:       time.Minute,
		MaxBodyBytes:          1024 * 1024,
		AllowLegacySignatures: true,
		MemorySoftCap:         1000,
	}
	s := &Server{
		cfg:          cfg,
		ipLimiter:    NewRateLimiter(cfg.RateLimitMax, cfg.RateLimitWindow),
		agentLimiter: NewRateLimiter(cfg.RateLimitMax, cfg.RateLimitWindow),
		promHandler:  promhttp.Handler(),
	}
	return s, s.Handler()
}

func TestMetricsEndpoint(t *testing.T) {
	const validToken = "12345678901234567890123456789012" // 32 characters

	t.Run("open when METRICS_TOKEN unset", func(t *testing.T) {
		_, handler := newTestMetricsServer(t, "", 100)

		for _, path := range []string{"/metrics", "/stats/cache"} {
			r := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200 OK for %s when METRICS_TOKEN is unset, got %d", path, w.Code)
			}
		}
	})

	t.Run("rejected when METRICS_TOKEN set and auth missing", func(t *testing.T) {
		_, handler := newTestMetricsServer(t, validToken, 100)

		for _, path := range []string{"/metrics", "/stats/cache"} {
			r := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 Unauthorized for %s, got %d", path, w.Code)
			}
			if got := w.Header().Get("WWW-Authenticate"); got != `Bearer realm="metrics"` {
				t.Fatalf("expected WWW-Authenticate header on %s, got %q", path, got)
			}
		}
	})

	t.Run("rejected when METRICS_TOKEN set and token invalid", func(t *testing.T) {
		_, handler := newTestMetricsServer(t, validToken, 100)

		for _, path := range []string{"/metrics", "/stats/cache"} {
			r := httptest.NewRequest("GET", path, nil)
			r.Header.Set("Authorization", "Bearer wrong-token-12345678901234567890")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 Unauthorized for %s with bad token, got %d", path, w.Code)
			}
		}
	})

	t.Run("accepted when METRICS_TOKEN matches bearer token", func(t *testing.T) {
		_, handler := newTestMetricsServer(t, validToken, 100)

		for _, path := range []string{"/metrics", "/stats/cache"} {
			r := httptest.NewRequest("GET", path, nil)
			r.Header.Set("Authorization", "Bearer "+validToken)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200 OK for %s with valid token, got %d (body: %s)", path, w.Code, w.Body.String())
			}
		}
	})

	t.Run("repeated bad tokens eventually get 429", func(t *testing.T) {
		_, handler := newTestMetricsServer(t, validToken, 3)

		// 3 allowed requests that fail auth
		for i := 1; i <= 3; i++ {
			r := httptest.NewRequest("GET", "/metrics", nil)
			r.RemoteAddr = "192.0.2.1:1234"
			r.Header.Set("Authorization", "Bearer invalid-token-guess")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("attempt %d: expected 401, got %d", i, w.Code)
			}
		}

		// 4th request must be rejected by rate limiter with 429
		r := httptest.NewRequest("GET", "/metrics", nil)
		r.RemoteAddr = "192.0.2.1:1234"
		r.Header.Set("Authorization", "Bearer invalid-token-guess")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429 Too Many Requests after exhausting rate limit, got %d", w.Code)
		}

		// Valid token request from the same IP must still be exempt and return 200
		rValid := httptest.NewRequest("GET", "/metrics", nil)
		rValid.RemoteAddr = "192.0.2.1:1234"
		rValid.Header.Set("Authorization", "Bearer "+validToken)
		wValid := httptest.NewRecorder()
		handler.ServeHTTP(wValid, rValid)

		if wValid.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for valid token even when IP rate limit is exhausted, got %d", wValid.Code)
		}
	})

	t.Run("short token makes config validation fail", func(t *testing.T) {
		cfgShort := config.Config{MetricsToken: "short-token"}
		if err := cfgShort.Validate(); err == nil {
			t.Fatal("expected config.Validate() to fail for token shorter than 32 characters")
		}

		cfgExact := config.Config{MetricsToken: "12345678901234567890123456789012"}
		if err := cfgExact.Validate(); err != nil {
			t.Fatalf("expected config.Validate() to succeed for 32 character token, got: %v", err)
		}

		cfgUnset := config.Config{MetricsToken: ""}
		if err := cfgUnset.Validate(); err != nil {
			t.Fatalf("expected config.Validate() to succeed when token unset, got: %v", err)
		}
	})
}
