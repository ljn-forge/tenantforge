# Installation

## Container installation

Run `python3 scripts/init-env.py`, then `docker compose --profile app up -d --build --wait`. The initializer exclusively creates `.env` with mode 0600 and preserves an existing file. The migrate container holds owner credentials; the application container receives only the runtime database account. Compose binds web, database and Mailpit ports to loopback.

Open `http://localhost:8090`. First startup atomically creates the configured superadministrator and default tenant. The local defaults are username `admin`, password `Admin@123456`, email `admin@tenantforge.local`. Initial password change is required before managing business data. In User management, add usernames, passwords, department and role assignments directly. Invitation email remains an optional workflow. Initial-account settings never overwrite an existing administrator.

Do not change database passwords in `.env` after the database volume is initialized without rotating the corresponding PostgreSQL roles. Updating an environment file alone does not update existing database roles.

## Native development

`make init` starts PostgreSQL/Mailpit, `make deps` installs locked dependencies, and `make dev` migrates the database and runs Go/Vite together. Ctrl-C stops both development processes. `make backend` and `make web` can run in separate terminals. Use Go 1.26.8 or a supported patched release and Node 24.21+. The browser uses Vite's same-origin API proxy.

## Environment

| Variable | Purpose |
| --- | --- |
| TF_DATABASE_URL | Runtime PostgreSQL URL; role must not own protected tables, inherit an owner/bypass role, be superuser or bypass RLS. |
| TF_MIGRATION_DATABASE_URL | Owner URL, only supplied to the migration process. |
| TF_MASTER_KEY | Base64-encoded 32-byte key encrypting MFA/OIDC/outbox material. Preserve it in backups. |
| TF_LISTEN | Native bind address, default 127.0.0.1:8090. |
| TF_PUBLIC_URL | Browser-visible origin used by CSRF and email/OIDC links. Native development uses http://localhost:5178. |
| TF_CONTAINER_PUBLIC_URL | Compose-specific browser origin, default http://localhost:8090. |
| TF_COOKIE_SECURE | true for an HTTPS deployment; requires HTTPS public URL. |
| TF_DATA_DIR / TF_WEB_DIR | Local object data and bundled static frontend directory. |
| TF_STORAGE_BACKEND | local (default) or s3. |
| TF_INITIAL_ADMIN_USERNAME / PASSWORD / EMAIL | Empty-database superadministrator credentials; configure them before first startup. |
| TF_INITIAL_TENANT_NAME | Automatically provisioned default tenant name. |
| TF_SMTP_HOST / PORT / FROM / USER / PASSWORD | Native SMTP delivery settings. Compose uses TF_CONTAINER_SMTP_HOST/PORT, default mailpit:1025. |
| TF_OIDC_ISSUER / CLIENT_ID / CLIENT_SECRET | Optional OIDC discovery issuer and registered client credentials. |
| TF_S3_ENDPOINT / REGION / BUCKET / ACCESS_KEY / SECRET_KEY | Optional S3-compatible endpoint and bucket; AWS credentials can also use the SDK default provider chain. |

Run `python3 scripts/run-env.py go run ./cmd/tenantforge check-config` to validate configuration without printing credentials. `version` needs no environment configuration. `migrate` is idempotent and uses a PostgreSQL session lock. `serve` refuses a database migration mismatch.

## HTTPS deployment

Use a TLS reverse proxy in front of port 8090. Set `TF_CONTAINER_PUBLIC_URL=https://your-domain.example` and `TF_COOKIE_SECURE=true`; use the exact same host in the browser. Configure a real authenticated SMTP provider. Mailpit is a local development mailbox. Keep migration credentials out of the application runtime. Store secrets in the deployment secret manager and protect database/object storage with network access controls.

## OIDC

Register redirect URI `<public-origin>/api/v1/auth/oidc/callback`. Configure issuer/client ID/client secret. The provider must supply a verified email for initial account linking; subsequent sign-ins bind the issuer/subject identity. State, nonce, PKCE, issuer, audience and signatures are verified. Local password/MFA login remains available.

## Object storage

Set storage backend to s3 and configure an existing bucket and region. For compatible providers set the endpoint. The adapter uses AWS SDK v2 and path-style addressing. Grant only bucket/object permissions required for put/get/delete. The platform stores opaque generated object keys and authorizes every download through the application. Protect bucket access; public bucket URLs bypass application authorization.

Choose the storage backend before uploading files. Changing providers does not migrate already stored objects; move objects under the same keys and verify them before switching configuration.
