-- +goose Up
ALTER TABLE api_keys ALTER COLUMN key_prefix TYPE VARCHAR(32);

-- +goose Down
ALTER TABLE api_keys ALTER COLUMN key_prefix TYPE VARCHAR(16);
