-- +goose Up
CREATE TABLE security_events(id uuid PRIMARY KEY,user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,action text NOT NULL,request_id text NOT NULL,changes jsonb NOT NULL DEFAULT '{}',created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX security_events_user ON security_events(user_id,created_at DESC);
CREATE INDEX sessions_expiry ON sessions(expires_at);
CREATE INDEX invitations_expiry ON invitations(tenant_id,expires_at);
CREATE INDEX leases_due ON job_leases(next_attempt_at,lease_until);
CREATE INDEX mail_due ON mail_outbox(next_attempt_at) WHERE status='pending';
ALTER TABLE invitation_tokens ADD CONSTRAINT invitation_token_reference FOREIGN KEY(token_hash) REFERENCES invitations(token_hash) ON DELETE CASCADE;
GRANT SELECT,INSERT ON security_events TO tenantforge_app;
