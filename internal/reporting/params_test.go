package reporting

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseDateRange(t *testing.T) {
	t.Run("valid YYYY-MM-DD range", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/summary?from=2026-10-01&to=2026-10-27", nil)
		from, to, err := parseDateRange(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedFrom := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		expectedTo := time.Date(2026, 10, 27, 23, 59, 59, 999999999, time.UTC)

		if !from.Equal(expectedFrom) {
			t.Errorf("expected from %v, got %v", expectedFrom, from)
		}
		if !to.Equal(expectedTo) {
			t.Errorf("expected to %v, got %v", expectedTo, to)
		}
	})

	t.Run("valid RFC3339 range", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/summary?from=2026-10-01T10:00:00Z&to=2026-10-27T18:00:00Z", nil)
		from, to, err := parseDateRange(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedFrom := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
		expectedTo := time.Date(2026, 10, 27, 18, 0, 0, 0, time.UTC)

		if !from.Equal(expectedFrom) {
			t.Errorf("expected from %v, got %v", expectedFrom, from)
		}
		if !to.Equal(expectedTo) {
			t.Errorf("expected to %v, got %v", expectedTo, to)
		}
	})

	t.Run("from after to returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/summary?from=2026-10-28&to=2026-10-27", nil)
		_, _, err := parseDateRange(req)
		if err == nil {
			t.Fatal("expected error for from > to, got nil")
		}
	})

	t.Run("malformed date returns error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/summary?from=not-a-date", nil)
		_, _, err := parseDateRange(req)
		if err == nil {
			t.Fatal("expected error for malformed date, got nil")
		}
	})

	t.Run("default 30-day window when omitted", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/summary", nil)
		from, to, err := parseDateRange(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if from.After(to) {
			t.Errorf("from %v should be before to %v", from, to)
		}
		diff := to.Sub(from)
		if diff < 29*24*time.Hour || diff > 31*24*time.Hour {
			t.Errorf("expected ~30 days difference, got %v", diff)
		}
	})
}

func TestParseLimit(t *testing.T) {
	t.Run("default limit", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/requests", nil)
		l, err := parseLimit(req, 20, 100)
		if err != nil || l != 20 {
			t.Fatalf("expected 20, got %d (err: %v)", l, err)
		}
	})

	t.Run("valid custom limit", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/requests?limit=50", nil)
		l, err := parseLimit(req, 20, 100)
		if err != nil || l != 50 {
			t.Fatalf("expected 50, got %d (err: %v)", l, err)
		}
	})

	t.Run("limit out of bounds", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/requests?limit=150", nil)
		_, err := parseLimit(req, 20, 100)
		if err == nil {
			t.Fatal("expected error for limit > 100")
		}

		reqZero := httptest.NewRequest("GET", "/api/v1/requests?limit=0", nil)
		_, errZero := parseLimit(reqZero, 20, 100)
		if errZero == nil {
			t.Fatal("expected error for limit < 1")
		}
	})

	t.Run("non-numeric limit", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/requests?limit=abc", nil)
		_, err := parseLimit(req, 20, 100)
		if err == nil {
			t.Fatal("expected error for non-numeric limit")
		}
	})
}

func TestFormatUSD(t *testing.T) {
	tests := []struct {
		micros   int64
		expected string
	}{
		{0, "0.000000"},
		{1, "0.000001"},
		{88, "0.000088"},
		{324510, "0.324510"},
		{1000000, "1.000000"},
		{2500000, "2.500000"},
		{-500000, "-0.500000"},
	}

	for _, tt := range tests {
		got := formatUSD(tt.micros)
		if got != tt.expected {
			t.Errorf("formatUSD(%d) = %q, expected %q", tt.micros, got, tt.expected)
		}
	}
}

func TestCursor_EncodeDecode(t *testing.T) {
	tm := time.Date(2026, 10, 27, 10, 14, 22, 123456000, time.UTC)
	id := "e229d7df-ec9c-4f17-91a6-fb69824c0d12"

	encoded := EncodeCursor(tm, id)
	decoded, err := DecodeCursor(encoded)
	if err != nil {
		t.Fatalf("failed to decode cursor: %v", err)
	}

	if decoded.ID != id {
		t.Errorf("expected ID %s, got %s", id, decoded.ID)
	}
	if !decoded.CreatedAt.Equal(tm) {
		t.Errorf("expected time %v, got %v", tm, decoded.CreatedAt)
	}

	// Test malformed cursor
	_, errMalformed := DecodeCursor("not-valid-base64!")
	if errMalformed == nil {
		t.Fatal("expected error for malformed cursor")
	}
}
