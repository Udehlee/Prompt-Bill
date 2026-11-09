package reporting

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// parseDateRange extracts and validates UTC 'from' and 'to' query parameters.
func parseDateRange(r *http.Request) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	to := now
	from := now.AddDate(0, 0, -30) // Default 30-day window

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	if toStr != "" {
		parsedTo, err := parseTimestamp(toStr, true)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid 'to' parameter: %w", err)
		}
		to = parsedTo
	}

	if fromStr != "" {
		parsedFrom, err := parseTimestamp(fromStr, false)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid 'from' parameter: %w", err)
		}
		from = parsedFrom
	} else if toStr != "" {
		// If only 'to' was specified, default 'from' to 30 days prior to 'to'
		from = to.AddDate(0, 0, -30)
	}

	if from.After(to) {
		return time.Time{}, time.Time{}, errors.New("invalid date range: 'from' must be before or equal to 'to'")
	}

	return from, to, nil
}

func parseTimestamp(val string, isEndOfDay bool) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", val); err == nil {
		if isEndOfDay {
			return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC), nil
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
	}

	if t, err := time.Parse(time.RFC3339Nano, val); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, val); err == nil {
		return t.UTC(), nil
	}

	return time.Time{}, errors.New("expected YYYY-MM-DD or RFC3339 timestamp")
}

// parseLimit extracts and bounds an integer limit query parameter.
func parseLimit(r *http.Request, defaultLimit, maxLimit int) (int, error) {
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		return defaultLimit, nil
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > maxLimit {
		return 0, fmt.Errorf("'limit' must be an integer between 1 and %d", maxLimit)
	}

	return limit, nil
}

// formatUSD converts integer microdollars into a formatted fixed-point USD decimal string ($0.000000).
func formatUSD(microdollars int64) string {
	sign := ""
	if microdollars < 0 {
		sign = "-"
		microdollars = -microdollars
	}
	dollars := microdollars / 1_000_000
	micros := microdollars % 1_000_000
	return fmt.Sprintf("%s%d.%06d", sign, dollars, micros)
}

// formatUSDPtr returns a formatted USD string pointer or nil if microdollars is nil.
func formatUSDPtr(microdollars *int64) *string {
	if microdollars == nil {
		return nil
	}
	s := formatUSD(*microdollars)
	return &s
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{
		Error:   code,
		Message: message,
	})
}
