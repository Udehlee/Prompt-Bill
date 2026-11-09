package reporting

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

// Cursor holds pagination state for stable, indexed keyset pagination.
type Cursor struct {
	CreatedAt time.Time `json:"t"`
	ID        string    `json:"id"`
}

// EncodeCursor serializes and base64-encodes a cursor.
func EncodeCursor(t time.Time, id string) string {
	raw, _ := json.Marshal(Cursor{CreatedAt: t.UTC(), ID: id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodeCursor parses a base64-encoded cursor string into a Cursor struct.
func DecodeCursor(encoded string) (*Cursor, error) {
	if encoded == "" {
		return nil, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		// Fallback to standard base64 if client passed padding
		data, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, errors.New("invalid base64 encoding")
		}
	}

	var c Cursor
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, errors.New("invalid cursor payload")
	}
	if c.ID == "" || c.CreatedAt.IsZero() {
		return nil, errors.New("cursor missing required fields")
	}
	c.CreatedAt = c.CreatedAt.UTC()
	return &c, nil
}
