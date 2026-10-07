# Dependency decisions

Evaluated on 2026-10-07 using repository-local patterns, npm metadata and upstream documentation. No private application source or integration configuration copied. Exact Go/npm versions are locked in go.sum and web/package-lock.json; deployed base images use digest pins.

| Need | Compared | Decision |
| --- | --- | --- |
| Existing administration systems | Gin Vue Admin, Go Admin, Vue Vben Admin | Use these as product/architecture references. TenantForge has a coherent per-operation tenant/data/field contract and its own integration tests; do not duplicate their generators or copy private app code. |
| Authorization | Casbin, OpenFGA, OPA | Casbin is an embeddable Go library supporting tenant-scoped roles. OpenFGA's relation-oriented service and OPA's general Rego policy engine suit broader distributed policy requirements; the approved product uses the smaller embedded role model plus validated SQL record predicates. |
| Database access | pgx, database/sql, GORM | pgx for explicit parameterized PostgreSQL queries, transactions, JSONB and runtime RLS; SQL predicates remain reviewable. |
| Migration | Goose, golang-migrate | Goose provider with embedded ordered migrations and PostgreSQL session locking. Supports both SQL and Go migrations. |
| Vue components | Element Plus, Naive UI, Ant Design Vue | Element Plus: TypeScript, complete administration components and local proven stack fit. Reuse its forms/tables/dialogs instead of implementing controls. |
| Identity protocols | coreos/go-oidc, custom JWT verification | Established OIDC discovery, issuer/audience/signature checks plus oauth2 state/nonce/PKCE. No custom JWT verification. |
| MFA | pquerna/otp, custom TOTP | Reuse tested TOTP implementation; platform owns enrollment lifecycle, encryption, replay/recovery protections. |
| Object storage | AWS SDK v2, hand-written S3 HTTP signing | AWS SDK v2 for configured S3-compatible providers; default local storage works offline. |
| Background jobs | Existing PostgreSQL lease pipeline, River, Asynq | Reuse the project’s tested transactional claims, publication fences and revocation predicates. Separate bounded Go worker lanes fix head-of-line blocking without replacing persisted jobs. River is a maintained PostgreSQL queue (MPL-2.0); Asynq is a Redis queue (MIT). A new platform can use River; this upgrade preserves the existing job history and avoids an additional Redis dependency. |
| Metrics | Existing counters, Prometheus Go client, OpenTelemetry SDK | Prometheus client provides maintained histogram/counter/gauge collectors and exposition. Use a private registry with bounded route/status/lane labels; never tenant/user identifiers. OpenTelemetry is appropriate when distributed tracing is required; the current single service needs direct scrape metrics. |
| Navigation icons | Installed Element Plus icons, custom SVG set | Reuse the existing Element Plus icon package with the current component library, rather than invent a second visual system. |
| Recovery | pg_dump/pg_restore, custom serialization | Official PostgreSQL tools plus private safe tar snapshot of local objects/configuration. |

Upstream sources:

- [Gin Vue Admin](https://github.com/flipped-aurora/gin-vue-admin), [Go Admin](https://github.com/go-admin-team/go-admin), [Vben Admin](https://github.com/vbenjs/vue-vben-admin).
- [Casbin tenant RBAC](https://casbin.apache.org/docs/rbac-with-domains/), [OpenFGA](https://github.com/openfga/openfga), [OPA](https://github.com/open-policy-agent/opa).
- [pgx](https://github.com/jackc/pgx), [Goose](https://github.com/pressly/goose), [golang-migrate](https://github.com/golang-migrate/migrate).
- [Element Plus](https://github.com/element-plus/element-plus), [Naive UI](https://github.com/tusen-ai/naive-ui), [Ant Design Vue](https://github.com/vueComponent/ant-design-vue).
- [River](https://github.com/riverqueue/river), [Asynq](https://github.com/hibiken/asynq), [Prometheus Go client](https://github.com/prometheus/client_golang), [Prometheus instrumentation practices](https://prometheus.io/docs/practices/instrumentation/), [Element Plus icons](https://github.com/element-plus/element-plus-icons).
- [PostgreSQL row-lock semantics](https://www.postgresql.org/docs/current/explicit-locking.html): authorization/revocation locks remain in short transactions; object transfers do not retain database connections. A separate quota lock avoids upgrading a shared tenant lock under concurrent requests.
- [OIDC](https://github.com/coreos/go-oidc), [OTP](https://github.com/pquerna/otp), [AWS Go SDK](https://github.com/aws/aws-sdk-go-v2).

License handling: TenantForge is MIT. Vue/Element Plus/Goose use MIT; Casbin/OpenFGA/OPA/go-oidc/AWS SDK use Apache-2.0; pgx and Element Plus icons use MIT; pquerna/otp and Prometheus Go client use Apache-2.0. [Third-party notices](../THIRD_PARTY_NOTICES.md) retain original texts from locked runtime modules and the installed npm production dependency graph, including tool peers; they are copied into the application image. The published lodash-unified package declares MIT but omits a separate license text, so its upstream metadata is recorded explicitly. Review and refresh the inventory when updating dependencies. Dependency adoption does not itself prove an integration secure; the verification matrix records checks on this application.
