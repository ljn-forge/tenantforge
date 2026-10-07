# TenantForge

User approved a complete, extensible, installable multi-tenant administration platform. No demonstration-only delivery or silently deferred requirements.

Use existing code, standard library, then maintained libraries before custom mechanisms. No copying private application code or configuration. Preserve module boundaries. Never print credentials, session tokens, reset tokens, MFA secrets, or credential-bearing DSNs.

The canonical contract is docs/api-contract.md. Coordinate contract changes before changing consumers. Backend files belong to the backend implementer; web files belong to the frontend implementer; root deployment, docs and end-to-end tests belong to the coordinator. Do not commit another worker's files.

Authorization is tenant AND operation AND record scope AND field access. Default deny. Only roles granting the current operation contribute scopes. Read, count, export, bulk writes and jobs use the same authorization. Revalidate membership and grants at execution. Writes and audit commit together. Optimistic versions and tenant-aware foreign keys are mandatory. Database runtime role must not own protected tables or have BYPASSRLS.

Write meaningful failing tests before security/domain behavior, then implement and verify. Record acceptance evidence; never call an unverified feature production-ready. No publishing or messaging to external accounts unless authorized for this project.
