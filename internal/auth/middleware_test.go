package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockResolver implements KeyResolver for unit testing.
type mockResolver struct {
	keys map[string]*mockKeyRecord
}

type mockKeyRecord struct {
	identity  CallerIdentity
	hash      string
	isRevoked bool
}

func (m *mockResolver) ResolveKey(ctx context.Context, prefix, plaintext string) (*CallerIdentity, error) {
	rec, exists := m.keys[prefix]
	if !exists {
		return nil, ErrKeyNotFound
	}

	if rec.isRevoked {
		return nil, ErrKeyRevoked
	}

	if !VerifyHash(plaintext, rec.hash) {
		return nil, ErrKeyNotFound
	}

	idCopy := rec.identity
	return &idCopy, nil
}

func TestMiddleware_AuthenticationFlow(t *testing.T) {
	// Provision two distinct tenants and keys in mock resolver
	keyA, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key A: %v", err)
	}

	keyB, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key B: %v", err)
	}

	revokedKey, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate revoked key: %v", err)
	}

	resolver := &mockResolver{
		keys: map[string]*mockKeyRecord{
			keyA.Prefix: {
				identity: CallerIdentity{
					TenantID:  "tenant-uuid-aaa",
					APIKeyID:  "key-uuid-aaa",
					KeyPrefix: keyA.Prefix,
				},
				hash: keyA.Hash,
			},
			keyB.Prefix: {
				identity: CallerIdentity{
					TenantID:  "tenant-uuid-bbb",
					APIKeyID:  "key-uuid-bbb",
					KeyPrefix: keyB.Prefix,
				},
				hash: keyB.Hash,
			},
			revokedKey.Prefix: {
				identity: CallerIdentity{
					TenantID:  "tenant-uuid-revoked",
					APIKeyID:  "key-uuid-revoked",
					KeyPrefix: revokedKey.Prefix,
				},
				hash:      revokedKey.Hash,
				isRevoked: true,
			},
		},
	}

	var capturedIdentity CallerIdentity
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := IdentityFromContext(r.Context())
		if !ok {
			http.Error(w, "missing identity in context", http.StatusInternalServerError)
			return
		}
		capturedIdentity = id
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	middleware := Middleware(resolver)
	server := middleware(testHandler)

	tests := []struct {
		name               string
		urlPath            string
		authHeader         string
		trackerKeyHeader   string
		extraHeaders       map[string]string
		expectStatus       int
		expectTenantID     string
		expectErrorMessage string
	}{
		{
			name:               "missing authorization",
			authHeader:         "",
			expectStatus:       http.StatusUnauthorized,
			expectErrorMessage: "missing or malformed authorization header",
		},
		{
			name:               "malformed format not bearer",
			authHeader:         "Basic dXNlcjpwYXNz",
			expectStatus:       http.StatusUnauthorized,
			expectErrorMessage: "missing or malformed authorization header",
		},
		{
			name:               "invalid prefix scheme",
			authHeader:         "Bearer sk-invalid-key-format",
			expectStatus:       http.StatusUnauthorized,
			expectErrorMessage: "invalid api key format",
		},
		{
			name:               "unknown key",
			authHeader:         "Bearer trk_live_00000000_nonexistentkey1234567890",
			expectStatus:       http.StatusUnauthorized,
			expectErrorMessage: "invalid api key",
		},
		{
			name:               "revoked key",
			authHeader:         "Bearer " + revokedKey.Plaintext,
			expectStatus:       http.StatusUnauthorized,
			expectErrorMessage: "api key has been revoked",
		},
		{
			name:           "valid key tenant A",
			authHeader:     "Bearer " + keyA.Plaintext,
			expectStatus:   http.StatusOK,
			expectTenantID: "tenant-uuid-aaa",
		},
		{
			name:           "valid key tenant B",
			authHeader:     "Bearer " + keyB.Plaintext,
			expectStatus:   http.StatusOK,
			expectTenantID: "tenant-uuid-bbb",
		},
		{
			name:             "valid key via X-Tracker-Key header",
			trackerKeyHeader: keyA.Plaintext,
			expectStatus:     http.StatusOK,
			expectTenantID:   "tenant-uuid-aaa",
		},
		{
			name:           "valid key via query parameter",
			urlPath:        "/v1/chat/completions?key=" + keyA.Plaintext,
			expectStatus:   http.StatusOK,
			expectTenantID: "tenant-uuid-aaa",
		},
		{
			name:       "caller cannot spoof tenant ID via custom header",
			authHeader: "Bearer " + keyA.Plaintext,
			extraHeaders: map[string]string{
				"X-Tracker-Tenant-Id": "tenant-uuid-bbb", // Attacker trying to spoof tenant B
				"X-Tracker-User-Id":   "user_42",
				"X-Tracker-Feature":   "search_v2",
			},
			expectStatus:   http.StatusOK,
			expectTenantID: "tenant-uuid-aaa", // Must resolve strictly to tenant A!
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturedIdentity = CallerIdentity{}
			targetURL := "/v1/chat/completions"
			if tt.urlPath != "" {
				targetURL = tt.urlPath
			}
			req := httptest.NewRequest(http.MethodGet, targetURL, nil)

			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.trackerKeyHeader != "" {
				req.Header.Set("X-Tracker-Key", tt.trackerKeyHeader)
			}
			for k, v := range tt.extraHeaders {
				req.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)

			if rec.Code != tt.expectStatus {
				t.Fatalf("expected HTTP status %d, got %d with body: %s", tt.expectStatus, rec.Code, rec.Body.String())
			}

			if tt.expectStatus == http.StatusOK {
				if capturedIdentity.TenantID != tt.expectTenantID {
					t.Fatalf("expected tenant ID %q, got %q", tt.expectTenantID, capturedIdentity.TenantID)
				}
				if tt.extraHeaders != nil {
					if capturedIdentity.UserID != tt.extraHeaders["X-Tracker-User-Id"] {
						t.Errorf("expected UserID %q, got %q", tt.extraHeaders["X-Tracker-User-Id"], capturedIdentity.UserID)
					}
					if capturedIdentity.Feature != tt.extraHeaders["X-Tracker-Feature"] {
						t.Errorf("expected Feature %q, got %q", tt.extraHeaders["X-Tracker-Feature"], capturedIdentity.Feature)
					}
				}
			} else {
				var errResp map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
					t.Fatalf("failed to parse JSON error: %v", err)
				}
				if errResp["message"] != tt.expectErrorMessage {
					t.Fatalf("expected error message %q, got %q", tt.expectErrorMessage, errResp["message"])
				}
			}
		})
	}
}
