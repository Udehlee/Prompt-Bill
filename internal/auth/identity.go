package auth

import (
	"context"
)

type contextKey struct{}
var identityKey = contextKey{}

// CallerIdentity holds the authenticated tenant identity and caller metadata
// TenantID and APIKeyID are verified from the API key stored in PostgreSQL
// UserID and Feature come from optional headers. callers cannot override TenantID
type CallerIdentity struct {
	TenantID  string
	APIKeyID  string
	KeyPrefix string
	UserID    string
	Feature   string
}

// ContextWithIdentity adds the authenticated CallerIdentity to the context.
func ContextWithIdentity(ctx context.Context, id CallerIdentity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// IdentityFromContext retrieves the CallerIdentity from context if authenticated
func IdentityFromContext(ctx context.Context) (CallerIdentity, bool) {
	id, ok := ctx.Value(identityKey).(CallerIdentity)
	return id, ok
}
