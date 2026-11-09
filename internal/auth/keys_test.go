package auth

import (
	"strings"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	if !strings.HasPrefix(key.Plaintext, KeySchemePrefix) {
		t.Fatalf("expected plaintext key to start with %q, got: %s", KeySchemePrefix, key.Plaintext)
	}

	if !strings.HasPrefix(key.Prefix, KeySchemePrefix) {
		t.Fatalf("expected key prefix to start with %q, got: %s", KeySchemePrefix, key.Prefix)
	}

	if len(key.Hash) != 64 {
		t.Fatalf("expected 64-char hex SHA-256 hash, got length %d", len(key.Hash))
	}

	// Verify constant-time verification
	if !VerifyHash(key.Plaintext, key.Hash) {
		t.Fatal("VerifyHash failed for generated key")
	}

	// Verify tampering detection
	tampered := key.Plaintext[:len(key.Plaintext)-2] + "xx"
	if VerifyHash(tampered, key.Hash) {
		t.Fatal("VerifyHash should fail for tampered key")
	}
}

func TestExtractPrefix(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantPrefix  string
		expectError bool
	}{
		{
			name:        "valid live key",
			input:       "trk_live_a1b2c3d4_8Fk9Lm2xYzaBcDeFgHiJkLmNoPqRsTuVwXyZ",
			wantPrefix:  "trk_live_a1b2c3d4",
			expectError: false,
		},
		{
			name:        "invalid prefix scheme",
			input:       "sk-openai-token-12345",
			wantPrefix:  "",
			expectError: true,
		},
		{
			name:        "truncated key",
			input:       "trk_live_short",
			wantPrefix:  "",
			expectError: true,
		},
		{
			name:        "missing secret",
			input:       "trk_live_a1b2c3d4_",
			wantPrefix:  "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, err := ExtractPrefix(tt.input)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tt.input, err)
				}
				if prefix != tt.wantPrefix {
					t.Fatalf("expected prefix %q, got %q", tt.wantPrefix, prefix)
				}
			}
		})
	}
}
