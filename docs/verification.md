# Verification evidence

## Follow-up checks on 2026-10-08

The extension task and loading-quality upgrade was checked against the same Go,
Node and PostgreSQL toolchain below. Existing private administrator credentials
were not reset. The stock binary keeps example modules opt-in.

| Check | Observed result |
| --- | --- |
| Project check | `make check` passed: Go race/unit tests, vet, formatting, 58 frontend tests in 18 files, type check, production build and gzip budgets |
| Full real PostgreSQL race suite | Passed: platform 43.765s, asset example 2.534s, extension 1.693s, migrations 1.479s |
| Production browser, stock binary | 3 passed in 27.4s; extension-only test intentionally skipped (`browser-check.py --built`) |
| Production browser, installed public extension | 4 passed in 37.4s (`browser-check.py --built --assets`); actual task creation/completion, bilingual typed results, viewport checks, mobile dark layout and styled message/confirmation services |
| Python tooling regressions | 19 passed, including static-import deduplication and resource-budget failures |
| REST specification | All 63 registered routes match OpenAPI; pinned Redocly 2.59.0 validation passed with the existing two OIDC redirect warnings |
| Frontend audit | Zero reported advisories including development dependencies; production dependency graph unchanged |
| Recovery | Eleven table fingerprints, object checksums, recovered encrypted MFA sign-in, permissions and authenticated file download passed against a fresh `tenantforge_restore_*` database |
| Container | Final application image built; container frontend tests and production build passed |
| Independent review | SPEC and QUALITY passed; directly identified jobs:create operation/department-scope gaps were closed with regressions |

The existing OIDC bounded-context test still expected 503 after the earlier
HTTP timeout classification changed to 504. It now asserts both 504 and the safe
`timeout` code while retaining the elapsed-time bound; focused and full database
race suites passed. No OIDC production behavior was changed in this follow-up.

Registration/input tests cover undeclared/required/type/enum/date values, the
custom validator, API-only compatibility and nil-versus-empty deep copies.
Authorization tests cover missing task-creation grants, missing business grants,
excluded and included department scopes, tenant membership and protected Owner
grant activation. Upgrade tests preserve custom role grants and administrator
password hashes and verify activation auditing is idempotent. Frontend tests
cover rejected inputs, duplicate submission, failed catalog retry and context
changes during loading/submission/inspection.

Gzip sizes are sums of independently compressed files at level 9, following the
recursive Vite static-import manifest closure. They are resource sizes, not
measured latency or a claim about proxy compression. The [complete file lists](assets/bundle-metrics.json)
include the shared chunks, rather than only the entry file.

| Profile | JavaScript before / after | CSS before / after |
| --- | --- | --- |
| Entry and static dependencies | 335807 / 122162 bytes | 52941 / 17258 bytes |
| Login and its dependencies | 338801 / 137491 bytes | 52941 / 19361 bytes |

The previous build exceeds the new 160 KiB JS / 25 KiB CSS budgets; the current
build passes. `make check` keeps this guard in CI. Visual evidence comes from the
installed public extension and persisted acceptance records:
[task creation](assets/extension-task-create-zh.png),
[Chinese typed results](assets/extension-task-zh.png), and
[mobile dark form](assets/extension-task-mobile-dark.png).

## Earlier checks on 2026-10-07

Observed locally on 2026-10-07 with Go 1.26.8, Node 24.21, PostgreSQL 18,
Mailpit and the pinned container images. These are application verification
results, not claims about a future deployment or long-term production use.

| Check | Observed result | Reproduce / evidence |
| --- | --- | --- |
| Go formatting, race tests, vet and compilation | Passed | `make check`; backend `go build ./...` |
| Real PostgreSQL integration and protocol tests | Passed with race detector: platform 39.897s, external asset example 2.457s, extension 2.040s, migrations 1.320s | `make integration`; `internal/platform/*_integration_test.go`, `protocol_test.go`, `examples/assets/assets_integration_test.go` |
| Vue unit/component tests | 49 tests in 17 files passed | `cd web && npm test` |
| Frontend formatting, TypeScript and production build | Passed | `npm run format:check`, `npm run build` in `web` |
| Fresh browser acceptance | 3 tests passed in 28.5s against fresh persisted PostgreSQL | `make browser`; `web/e2e/product.spec.ts`, `web/e2e/quality.spec.ts` |
| Python backup, isolation, environment and cancellation regressions | 15 tests passed | `python3 -m unittest discover -s tests -p 'test_*.py'` |
| OpenAPI contract | All 62 registered API operations match the specification; OpenAPI 3.1 validation passed | `docs/openapi.json`; Redocly CLI 2.59.0 with minimal rules |
| Container distribution | Final image built; live migration to 010 and readiness healthy at port 8090 | `docker compose --profile app build`; after stopping old app writes, `docker compose --profile app up -d --no-build --wait` |
| Default administrator installation | Fresh `admin` sign-in, required initial password change and restricted-user administration passed | Fresh isolated browser fixture; existing administrator password hash/state preserved by upgrade comparison |
| Runtime boundary | UID 10001; owner/migration/test credentials absent; PostgreSQL runtime role is neither superuser nor BYPASSRLS, owns no public tables; all eight declared tenant tables force RLS | Actual final container environment keys, PostgreSQL role/table flags inspected without printing secrets |
| Native backup and recovery | Eleven table fingerprints and every local object checksum match; recovered MFA sign-in, permissions and authenticated file download passed | `make recovery` after `make browser` |
| Container backup | Actual app stopped, PostgreSQL/custom dump and container data captured, app restarted healthy | `python3 scripts/backup.py` |
| SIGTERM during recovery | Earlier baseline drill: nonzero exit, browser MFA disabled, both detached verification servers stopped; current regressions passed | Earlier real interrupted drill plus `tests/test_recovery.py` startup-interruption regression |
| Browser reset with another active database role | Earlier baseline drill: reset refused and users preserved; unchanged all-role guard still covered by current regressions | Owner-role connection to `tenantforge_browser`; reset guard checks all client backends |
| npm dependency audit | Zero reported vulnerabilities, including development dependencies | `cd web && npm audit --audit-level=high` |
| Go vulnerability scan | Zero vulnerable called symbols or imported packages; one advisory in an unused dependency package | `govulncheck` v1.8.0; see scope below |
| Independent source review | Backend and frontend SPEC/QUALITY reviews passed, including tenant reload and cancellation addenda | `quality_integration_test.go`, `final_review_integration_test.go`, extension and frontend/Python suites |

Browser acceptance covers initial password change, direct administrator-created
users, restricted-user navigation, departments, roles and permission editing,
ticket lifecycle, CSV, upload/download, jobs, audit, MFA, tenant switching across
hard reloads, permission presets, mixed ownership, department clearing, Chinese
presentation, keyboard navigation and light/dark responsive layouts. Server integration tests exercise row/field authorization
through alternate access routes, cross-tenant isolation, target delegation,
concurrency/version conflicts, quotas, retry/cancellation, revocation and
publication-safe object cleanup. Channel-gated regressions additionally cover
slow upload/download without a held database connection, independent worker
lanes, concurrent reservations, authority narrowing during a custom job,
department-tree revocation, lowered export quota and cancellation while a record
lock is pending. HTTP cancellation/deadline classification has separate pure
race regressions; it no longer shares the generic internal-failure code.

The public asset entrypoint also compiled and ran `version` from a standalone
Go module outside this project, importing only public packages. Its integration
test verifies custom approval, expected version, audit and inventory job results.

Before changing the live schema, the existing container was backed up to a
private archive and restored into `tenantforge_restore_quality_20261007`.
Migration 006 to 010 and readiness passed. All eleven existing table fingerprints
matched the unchanged original database, including administrator password data;
the existing administrator had already changed the initial password and was
not reset.

The approved source was then fast-forwarded into local `main`. The old app was
stopped before migration, and the final image started successfully on port 8090.
Live health/readiness and the administrator login page passed; the latter has
no workspace creation wizard and produced no browser runtime errors. All eleven
existing table fingerprints still matched the pre-upgrade baseline. Final
`make check` in the merged checkout passed, including all 49 frontend tests.
The existing account password remains unchanged; fresh-install login is covered
by the separate browser fixture.

Visual artifacts use authorized, persisted acceptance fixtures, not synthetic
counts: [Chinese dashboard](assets/dashboard-zh.png),
[Chinese permission matrix](assets/roles-zh.png),
[record/field scope editor](assets/roles-grant-zh.png),
[dark dashboard](assets/dashboard-dark.png),
[mobile light](assets/mobile-light.png), [mobile dark](assets/mobile-dark.png)
and the [live administrator login](assets/login.png).
Final Chinese date formatting and mobile overflow were also checked in an
independent browser context after the cosmetic date change.

Recovery uses only `tenantforge_browser` and a fresh `tenantforge_restore_*`
database. Private archives, restored configuration and logs remain under ignored
`.backups`; the normal application database is not reset. Recovery temporarily
enrolls the fixture administrator in MFA, uses the recovered encrypted secret to
sign in, and disables fixture MFA during cleanup. Existing snapshot files and
restore databases are never overwritten.

OIDC tests use signed local RSA/JWKS discovery, issuer/audience/nonce/state,
PKCE and replay checks. S3 tests exercise the AWS SDK against a local HTTP
protocol fixture, including multipart seek/retry. SMTP delivery is read back
from local Mailpit. Real external identity providers, mail services and object
storage accounts require deployment-specific configuration and acceptance.

Redocly reports two advisory warnings because the OIDC start/callback operations
return HTTP 302 rather than 2xx; their documented redirects match the service.
The Go module advisory is GO-2026-5932 in unused `golang.org/x/crypto/openpgp`;
TenantForge neither imports nor calls that package. The scan therefore does not
claim every package present in a required module is advisory-free.

The GitHub Actions workflow includes tests, isolated browser/recovery runs,
dependency checks and image build. It is configured in this local repository;
a remote GitHub workflow run has not been performed.

```sh
make init
make deps
make check
make integration
make browser
make recovery
python3 -m unittest discover -s tests -p 'test_*.py'
cd web && npm audit --audit-level=high
```
