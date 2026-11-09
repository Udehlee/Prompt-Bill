package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Udehlee/Prompt-Bill/internal/auth"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// EnsureTenant finds an existing tenant by name 
// creates a new one if it doesn't exist
func (r *Repository) EnsureTenant(ctx context.Context, name string) (string, error) {
	var id string
	query := `SELECT id FROM tenants WHERE name = $1 LIMIT 1`
	err := r.db.QueryRowContext(ctx, query, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("failed to query tenant: %w", err)
	}

	insertQuery := `INSERT INTO tenants (name) VALUES ($1) RETURNING id`
	if err := r.db.QueryRowContext(ctx, insertQuery, name).Scan(&id); err != nil {
		return "", fmt.Errorf("failed to insert tenant: %w", err)
	}

	return id, nil
}

// ProvisionKey creates a new API key for a tenant, saves its prefix and hash,
// and returns the secret key to be shown only once.
func (r *Repository) ProvisionKey(ctx context.Context, tenantName, keyName string) (*auth.GeneratedKey, string, error) {
	tenantID, err := r.EnsureTenant(ctx, tenantName)
	if err != nil {
		return nil, "", err
	}

	key, err := auth.GenerateKey()
	if err != nil {
		return nil, "", err
	}

	insertQuery := `
		INSERT INTO api_keys (tenant_id, key_prefix, key_hash, name)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`
	var keyID string
	err = r.db.QueryRowContext(ctx, insertQuery, tenantID, key.Prefix, key.Hash, keyName).Scan(&keyID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to persist api key: %w", err)
	}

	return key, tenantID, nil
}

// ResolveKey finds an API key by its prefix and checks if the secret is valid.
// Returns an error if the key is not found, invalid, or revoked.
func (r *Repository) ResolveKey(ctx context.Context, prefix, plaintext string) (*auth.CallerIdentity, error) {
	query := `
		SELECT id, tenant_id, key_hash, revoked_at
		FROM api_keys
		WHERE key_prefix = $1
		LIMIT 1
	`
	var (
		keyID      string
		tenantID   string
		storedHash string
		revokedAt  *time.Time
	)

	err := r.db.QueryRowContext(ctx, query, prefix).Scan(&keyID, &tenantID, &storedHash, &revokedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, auth.ErrKeyNotFound
		}
		return nil, fmt.Errorf("failed to query api key: %w", err)
	}

	if revokedAt != nil {
		return nil, auth.ErrKeyRevoked
	}

	if !auth.VerifyHash(plaintext, storedHash) {
		return nil, auth.ErrKeyNotFound
	}

	return &auth.CallerIdentity{
		TenantID:  tenantID,
		APIKeyID:  keyID,
		KeyPrefix: prefix,
	}, nil
}

// RevokeKey marks an API key as revoked
func (r *Repository) RevokeKey(ctx context.Context, keyID string) error {
	query := `UPDATE api_keys SET revoked_at = clock_timestamp() WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, keyID)
	if err != nil {
		return fmt.Errorf("failed to revoke key: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return auth.ErrKeyNotFound
	}
	return nil
}
