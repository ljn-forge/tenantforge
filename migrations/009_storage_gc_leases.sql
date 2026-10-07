-- +goose Up
ALTER TABLE storage_gc ADD COLUMN claim_token text;
ALTER TABLE storage_gc ADD COLUMN lease_until timestamptz;
-- +goose Down
ALTER TABLE storage_gc DROP COLUMN claim_token, DROP COLUMN lease_until;
