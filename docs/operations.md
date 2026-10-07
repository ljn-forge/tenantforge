# Operations

## Health and supervision

`/healthz` reports process/version; `/readyz` checks database availability. `/metrics` exports request/rejection counters, request duration histograms, worker lane results and queue depth, and defaults to loopback access. Network admission controls are required if explicitly enabling public metrics. Compose supervises the application and uses readiness checks. Structured logs contain request IDs, operation/path and generic rejection codes.

Scrape metrics use registered route templates, HTTP status and fixed worker lane/stage/code labels. They contain no tenant/user IDs or raw URLs. `tenantforge_http_requests_total` and `tenantforge_http_request_duration_seconds` describe API requests; `tenantforge_worker_outcomes_total` and `tenantforge_worker_duration_seconds` describe lane work; `tenantforge_queue_backlog` reports durable queue items. The legacy request/rejection counters remain available.

Business jobs, email, object collection and housekeeping have independent bounded worker lanes and cancellation contexts. A slow export does not consume the mail or collection deadline. Shutdown cancels and waits for every lane. Typed error stage/code fields support diagnosis without exposing raw provider responses.

Session activity is coalesced asynchronously rather than locking the session row for an entire HTTP request. Uploads use short authorization/reservation and publication transactions around object storage I/O; concurrent reservations count toward the storage quota. Publication rechecks current authority and converts one reservation into committed usage. Expired reservations retain cleanup evidence across crashes. Downloads complete current authorization and audit before copying a response body, so a slow client does not hold a database connection.

Jobs and email delivery persist in PostgreSQL. Workers claim with leases and bounded retries. Running export cancellation is cooperative; a cancelled claim cannot publish a succeeded result. Membership, tenant status and permissions are revalidated. Download rechecks current permissions. SMTP delivery is at least once: a connection failure after an SMTP server accepted a message can result in a duplicate on retry.

## Local-storage backup

Backups are offline to keep database references and local objects consistent. Container backup stops/restarts the app; stop a native backend manually first. Existing snapshot files are never overwritten.

```sh
python3 scripts/backup.py --output .backups/snapshot.tar.gz
# Stopped native server:
python3 scripts/backup.py --native --output .backups/native-snapshot.tar.gz
```

Snapshots include a PostgreSQL custom-format dump, local data, configuration/encryption keys, inventory and SHA-256 checksums. Archives are private files; encrypt and restrict off-host copies. Losing TF_MASTER_KEY prevents recovery of encrypted MFA/outbox material. The script rejects active runtime database connections.

## Restore

Restore is deliberately non-destructive: choose a new database and new directory. It validates the archive paths, regular files and checksum inventory before creating the database. Existing databases/directories are rejected.

```sh
python3 scripts/backup.py --restore .backups/snapshot.tar.gz \
  --database tenantforge_restore_drill --destination .backups/restore-drill
```

The script writes a private `.env` under the restored directory with database URLs and local data pointing to the isolated restoration and port 8092. It disables SMTP, OIDC and S3 credentials, selects local storage and sets a loopback HTTP cookie policy so a drill cannot send queued production email or delete objects in a live bucket. Run the same application version with that environment, verify records/audit/file contents and sign-in, then explicitly configure integrations for a controlled cutover. It does not overwrite the live database. Clear inherited TF_* overrides when loading the restored configuration.

```sh
python3 scripts/run-env.py --env-file .backups/restore-drill/.env go run ./cmd/tenantforge serve
```

An explicitly selected environment file overrides inherited values for its declared keys, and the runtime wrapper removes owner/test/bootstrap database credentials. Use `--owner` only for the isolated migration command. The Compose snapshot helper supports its own PostgreSQL service and rejects connection-target query overrides or external clusters; back up external database deployments with their provider tools.

S3 objects are external to the local snapshot. Use the provider's versioning/replication/backup controls and restore the same object keys alongside the database. Test provider restore before relying on an S3 deployment.

For a repeatable local recovery drill, run `make browser` followed by `make recovery`. The drill snapshots only the isolated browser database and its objects, restores into a new database, compares eleven table fingerprints and every object checksum, and verifies encrypted MFA sign-in, permissions and an authenticated file download. It temporarily enables MFA on the browser fixture administrator and disables it during cleanup. Private archives and restored databases are retained under `.backups` for inspection. It never resets the main database.

## Upgrade

Take and verify a backup, stop app writes, pull/build the intended revision, run the separate migration service with owner credentials, then start the runtime service. Goose records ordered migrations and acquires a PostgreSQL session lock. Readiness refuses a schema mismatch. Test an upgrade against a restored database first. Restore the previous database/object snapshot to roll back a data-changing migration; do not assume an old binary can run against a new schema.

## Secrets

Do not commit `.env`, snapshots or restored configuration. Keep the master key stable across application restarts. Password rotation must update the PostgreSQL role and application/migration secrets together. Platform secrets never belong in the frontend bundle.
