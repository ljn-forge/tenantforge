-- +goose Up
-- +goose StatementBegin
CREATE TABLE job_record_effects(tenant_id uuid NOT NULL REFERENCES tenants(id),job_id uuid NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,record_id uuid NOT NULL,action text NOT NULL,input_hash text NOT NULL,PRIMARY KEY(job_id,record_id,action,input_hash));
ALTER TABLE job_record_effects ENABLE ROW LEVEL SECURITY;
ALTER TABLE job_record_effects FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON job_record_effects USING(tenant_id::text=current_setting('app.tenant_id',true)) WITH CHECK(tenant_id::text=current_setting('app.tenant_id',true));
GRANT SELECT,INSERT,UPDATE,DELETE ON job_record_effects TO tenantforge_app;
-- +goose StatementEnd
