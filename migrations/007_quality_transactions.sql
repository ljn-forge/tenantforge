-- +goose Up
-- +goose StatementBegin
CREATE TABLE tenant_quota_locks(tenant_id uuid PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE);
INSERT INTO tenant_quota_locks SELECT id FROM tenants;
ALTER TABLE tenant_quota_locks ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_quota_locks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenant_quota_locks USING(tenant_id::text=current_setting('app.tenant_id',true)) WITH CHECK(tenant_id::text=current_setting('app.tenant_id',true));
ALTER TABLE storage_gc ADD COLUMN reservation_bytes bigint NOT NULL DEFAULT 0 CHECK(reservation_bytes>=0);
ALTER TABLE jobs ADD COLUMN error_code text NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN error_stage text NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN result jsonb NOT NULL DEFAULT '{}';
ALTER TABLE mail_outbox ADD COLUMN error_code text NOT NULL DEFAULT '';
ALTER TABLE mail_outbox ADD COLUMN error_stage text NOT NULL DEFAULT '';
ALTER TABLE storage_gc ADD COLUMN error_code text NOT NULL DEFAULT '';
ALTER TABLE storage_gc ADD COLUMN error_stage text NOT NULL DEFAULT '';
GRANT SELECT,INSERT,UPDATE,DELETE ON tenant_quota_locks TO tenantforge_app;
REVOKE INSERT,UPDATE,DELETE ON goose_db_version FROM tenantforge_app;
-- +goose StatementEnd
