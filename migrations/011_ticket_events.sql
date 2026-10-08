-- +goose Up
ALTER TABLE resources ADD CONSTRAINT resources_tenant_id_module_unique UNIQUE(tenant_id,id,module);
CREATE TABLE ticket_events (
 id uuid PRIMARY KEY,
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 ticket_id uuid NOT NULL,
 module text NOT NULL DEFAULT 'tickets' CHECK(module='tickets'),
 kind text NOT NULL CHECK(kind IN ('created','updated','assigned','status_changed','reply','note')),
 actor_id uuid,
 actor_name text NOT NULL DEFAULT '',
 body text,
 metadata jsonb NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,ticket_id,module) REFERENCES resources(tenant_id,id,module) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,actor_id) REFERENCES memberships(tenant_id,user_id) ON DELETE SET NULL(actor_id),
 CHECK(body IS NULL OR (octet_length(body)<=20000 AND kind IN ('reply','note','status_changed'))),
 CHECK(kind NOT IN ('reply','note') OR (body IS NOT NULL AND char_length(body)<=4000 AND octet_length(body)<=8192 AND length(btrim(body))>0))
);
CREATE INDEX ticket_events_page ON ticket_events(tenant_id,ticket_id,created_at DESC,id DESC);
ALTER TABLE ticket_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE ticket_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ticket_events USING(tenant_id::text=current_setting('app.tenant_id',true)) WITH CHECK(tenant_id::text=current_setting('app.tenant_id',true));
REVOKE ALL ON ticket_events FROM PUBLIC,tenantforge_app;
GRANT SELECT,INSERT ON ticket_events TO tenantforge_app;
