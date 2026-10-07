-- +goose Up
CREATE INDEX resources_asset_name ON resources(tenant_id, (data->>'name')) WHERE module='assets';

-- +goose Down
DROP INDEX resources_asset_name;
