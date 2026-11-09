package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// KeyResolver resolves a tracker API key to its authenticated tenant identity.
type KeyResolver interface {
	ResolveKey(ctx context.Context, prefix, plaintextKey string) (*CallerIdentity, error)
}

// Middleware creates an HTTP middleware that authenticates caller requests using tracker API keys.
func Middleware(resolver KeyResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				writeError(w, http.StatusUnauthorized, "missing or malformed authorization header")
				return
			}

			prefix, err := ExtractPrefix(token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid api key format")
				return
			}

			identity, err := resolver.ResolveKey(r.Context(), prefix, token)
			if err != nil {
				if errors.Is(err, ErrKeyRevoked) {
					writeError(w, http.StatusUnauthorized, "api key has been revoked")
					return
				}
				writeError(w, http.StatusUnauthorized, "invalid api key")
				return
			}

			identity.UserID = trimHeader(r.Header.Get("X-Tracker-User-Id"), 128)
			identity.Feature = trimHeader(r.Header.Get("X-Tracker-Feature"), 128)

			ctx := ContextWithIdentity(r.Context(), *identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	if keyHeader := r.Header.Get("X-Tracker-Key"); keyHeader != "" {
		return strings.TrimSpace(keyHeader)
	}

	if queryKey := r.URL.Query().Get("key"); queryKey != "" {
		return strings.TrimSpace(queryKey)
	}

	return ""
}

func trimHeader(val string, maxLen int) string {
	val = strings.TrimSpace(val)
	if len(val) > maxLen {
		return val[:maxLen]
	}
	return val
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "unauthorized",
		"message": message,
	})
}
