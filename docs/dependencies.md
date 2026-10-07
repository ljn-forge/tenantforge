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
| Background jobs | PostgreSQL leases, Redis-backed queue | Durable PostgreSQL claims/leases and bounded retries integrate transactionally with the authoritative store and require no additional mandatory broker. No custom message protocol. |
| Recovery | pg_dump/pg_restore, custom serialization | Official PostgreSQL tools plus private safe tar snapshot of local objects/configuration. |

Upstream sources:

- [Gin Vue Admin](https://github.com/flipped-aurora/gin-vue-admin), [Go Admin](https://github.com/go-admin-team/go-admin), [Vben Admin](https://github.com/vbenjs/vue-vben-admin).
- [Casbin tenant RBAC](https://casbin.apache.org/docs/rbac-with-domains/), [OpenFGA](https://github.com/openfga/openfga), [OPA](https://github.com/open-policy-agent/opa).
- [pgx](https://github.com/jackc/pgx), [Goose](https://github.com/pressly/goose), [golang-migrate](https://github.com/golang-migrate/migrate).
- [Element Plus](https://github.com/element-plus/element-plus), [Naive UI](https://github.com/tusen-ai/naive-ui), [Ant Design Vue](https://github.com/vueComponent/ant-design-vue).
- [OIDC](https://github.com/coreos/go-oidc), [OTP](https://github.com/pquerna/otp), [AWS Go SDK](https://github.com/aws/aws-sdk-go-v2).

License handling: TenantForge is MIT. Vue/Element Plus/Goose use MIT; Casbin/OpenFGA/OPA/go-oidc/AWS SDK use Apache-2.0; pgx uses MIT; pquerna/otp uses Apache-2.0. [Third-party notices](../THIRD_PARTY_NOTICES.md) retain original texts from locked runtime modules and the installed npm production dependency graph, including tool peers; they are copied into the application image. The published lodash-unified package declares MIT but omits a separate license text, so its upstream metadata is recorded explicitly. Review and refresh the inventory when updating dependencies. Dependency adoption does not itself prove an integration secure; the verification matrix records checks on this application.
