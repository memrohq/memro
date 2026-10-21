package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memrohq/memro/server/config"
)

func TestMetricsEndpoint(t *testing.T) {
	t.Run("open when METRICS_TOKEN unset", func(t *testing.T) {
		s := &Server{cfg: config.Config{MetricsToken: ""}}
		r := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()

		s.metrics(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK when METRICS_TOKEN is unset, got %d", w.Code)
		}
	})

	t.Run("rejected when METRICS_TOKEN set and auth missing", func(t *testing.T) {
		s := &Server{cfg: config.Config{MetricsToken: "secret-token"}}
		r := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()

		s.metrics(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
		}
		if got := w.Header().Get("WWW-Authenticate"); got != `Bearer realm="metrics"` {
			t.Fatalf("expected WWW-Authenticate header, got %q", got)
		}
	})

	t.Run("rejected when METRICS_TOKEN set and token invalid", func(t *testing.T) {
		s := &Server{cfg: config.Config{MetricsToken: "secret-token"}}
		r := httptest.NewRequest("GET", "/metrics", nil)
		r.Header.Set("Authorization", "Bearer wrong-token")
		w := httptest.NewRecorder()

		s.metrics(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", w.Code)
		}
	})

	t.Run("accepted when METRICS_TOKEN matches bearer token", func(t *testing.T) {
		s := &Server{cfg: config.Config{MetricsToken: "secret-token"}}
		r := httptest.NewRequest("GET", "/metrics", nil)
		r.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()

		s.metrics(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", w.Code)
		}
	})
}
