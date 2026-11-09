package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardHandler(t *testing.T) {
	h := NewHandler()

	t.Run("GET /dashboard returns 200 OK with html content", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d", rec.Code)
		}

		contentType := rec.Header().Get("Content-Type")
		if !strings.HasPrefix(contentType, "text/html") {
			t.Errorf("expected text/html content-type, got %s", contentType)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "Prompt-Bill") {
			t.Errorf("expected body to contain project title 'Prompt-Bill'")
		}
		if !strings.Contains(body, "Estimated Cost (USD)") {
			t.Errorf("expected body to contain Estimated Cost label")
		}
		if !strings.Contains(body, "style.css") {
			t.Errorf("expected body to link to style.css")
		}
		if !strings.Contains(body, "dashboard.js") {
			t.Errorf("expected body to link to dashboard.js")
		}
	})

	t.Run("GET /dashboard/style.css returns 200 OK with css content", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dashboard/style.css", nil)
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d", rec.Code)
		}

		contentType := rec.Header().Get("Content-Type")
		if !strings.HasPrefix(contentType, "text/css") {
			t.Errorf("expected text/css content-type, got %s", contentType)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "--accent:") {
			t.Errorf("expected css body to contain color variables")
		}
	})

	t.Run("GET /dashboard/dashboard.js returns 200 OK with js content", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dashboard/dashboard.js", nil)
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d", rec.Code)
		}

		contentType := rec.Header().Get("Content-Type")
		if !strings.HasPrefix(contentType, "application/javascript") {
			t.Errorf("expected application/javascript content-type, got %s", contentType)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "apiFetch") {
			t.Errorf("expected js body to contain apiFetch")
		}
	})

	t.Run("non-GET method returns 405 Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/dashboard", nil)
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected HTTP 405, got %d", rec.Code)
		}
	})
}
