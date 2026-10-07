-- +goose Up
-- +goose StatementBegin
ALTER TABLE storage_gc ADD COLUMN tenant_id uuid;
CREATE INDEX storage_gc_key_idx ON storage_gc(storage_key);
-- +goose StatementEnd
