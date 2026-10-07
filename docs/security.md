# Permission and security model

## Tenant boundary

Identity is global; tenant data requires an active membership and active tenant. `X-Tenant-ID` is verified per request. A platform administrator can provision/suspend tenants but gains no implicit access to tenant records. Runtime PostgreSQL RLS and tenant-aware references add a database boundary. The runtime account must be non-owner, non-superuser and non-bypass; migrations use a separate process/account.

## Grants

Every grant identifies a resource, action and scope. Matching grants are unioned only for that action. Conditions inside a grant are combined with AND; alternatives across grants use OR. Unknown resources/actions/fields/conditions are rejected.

| Scope | Records included |
| --- | --- |
| self | Records owned by the user. |
| department | Records in the member's current department. |
| department_tree | That department and descendants. |
| selected_departments | Explicitly selected tenant departments. |
| tenant | All records in the selected tenant. |

Reads and writes have distinct checks. Mutation authorization applies to both the existing and resulting row, expected version and relevant write fields. Moving a record needs assignment permission as well as update. Role administrators may delegate only within their effective authority. The last active tenant owner cannot be removed or suspended.

## Fields

Readable/writable/unmasked fields are checked against grants matching the individual row. `*` covers non-sensitive fields only. A named sensitive read field is masked unless explicitly listed in unmask_fields. Invisible fields are omitted. A broad read grant does not give permission to write protected fields. The role editor consumes the backend registry and the browser is never the authoritative authorization layer.

Record responses expose server-computed capabilities before data filtering. The browser defaults to denying actions when capabilities are absent. Update writable fields are separate from custom-operation field authority. Dashboard classification counts also apply field-read predicates so restricted readers cannot infer hidden status or priority.

Audit stores changed field names and ownership/version metadata rather than raw sensitive business values. CSV exports use current authorization and field filters, including authorization on download. Formula-like values are escaped. CSV import validates all rows in a bounded transaction; rejected imports commit no partial rows.

## Accounts and requests

Opaque session tokens are hashed server-side and kept in HttpOnly SameSite=Lax cookies; frontend storage contains only tenant selection. Mutation requests require session-bound CSRF tokens and an allowed origin. Membership/grant changes and revocation take effect on subsequent requests and job execution. MFA secrets and queued email payloads are encrypted by TF_MASTER_KEY; reset tokens and one-time recovery codes are hashed.

Production cookies require HTTPS configuration. Responses include CSP, frame denial, MIME sniffing protection, referrer policy and restrictive browser permissions. Error responses are generic and include request IDs; logs omit token-bearing query strings and raw backend errors.

## Reporting

For a vulnerability, share the affected version, safe reproduction and expected authorization result privately with the repository maintainer. Do not place credentials or real tenant data in public issues. Use a disposable environment for cross-tenant/security tests.
