-- +goose Up
ALTER TABLE jobs ADD COLUMN authority_hash text NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN authority_resource text NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN authority_action text NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE jobs DROP COLUMN authority_hash, DROP COLUMN authority_resource, DROP COLUMN authority_action;
