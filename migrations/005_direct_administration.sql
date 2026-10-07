-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN username text;
UPDATE users SET username='user_'||replace(id::text,'-','');
ALTER TABLE users ALTER COLUMN username SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_username_unique UNIQUE(username);
ALTER TABLE users ADD CONSTRAINT users_username_format CHECK(username ~ '^[a-z][a-z0-9_.-]{2,39}$');
ALTER TABLE users ADD COLUMN must_change_password boolean NOT NULL DEFAULT false;
DO $$ DECLARE tenant uuid; BEGIN FOR tenant IN SELECT id FROM tenants LOOP PERFORM set_config('app.tenant_id',tenant::text,true); UPDATE roles SET grants=grants || '[{"resource":"members","action":"create","scope":"tenant","read_fields":["*"],"write_fields":["*"]}]'::jsonb WHERE tenant_id=tenant AND builtin AND name='Owner' AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(grants) g WHERE g->>'resource'='members' AND g->>'action'='create'); END LOOP; END $$;
-- +goose StatementEnd
