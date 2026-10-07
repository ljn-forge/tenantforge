# REST/UI contract

Base `/api/v1`. JSON success `{data:T,meta?:{page:number,page_size:number,total:number}}`; error `{error:{code:string,message:string,fields?:object},request_id:string}`. UUID string IDs; UTC ISO timestamps; JSON snake_case. GET lists return `data:[]` with meta; page default1/page_size20(max100), `q`, optional module filters. Unknown JSON fields rejected for typed control payloads.

Cookie `tf_session` HttpOnly SameSite=Lax; CSRF token from GET `/auth/csrf` -> `{data:{csrf_token}}`, send `X-CSRF-Token` on mutations. Browser fetch credentials include. Tenant context `X-Tenant-ID` UUID on every scoped endpoint; server verifies membership/current status every request. Frontend stores selected tenant ID only (no auth tokens). Standard errors: 401 unauthorized,403 forbidden/mfa_required,404 not_found,409 conflict,422 validation,429 rate_limited.

## Setup and authentication

GET `/setup/status`: `{initialized:boolean,version:string,oidc_enabled:boolean}`.
POST `/setup`: `{email,password,display_name,tenant_name}` atomically initializes platform admin plus tenant owner, sets session. Initial password min12 chars.
POST `/auth/login`: `{email,password,totp?:string,recovery_code?:string}` -> current user; `mfa_required` if second factor needed.
POST `/auth/logout`: clears/revokes session.
GET `/auth/me`: `{user:User,tenants:Tenant[],permissions:Grant[],modules:Module[],platform_admin:boolean}`. With a selected tenant header it includes that tenant's capabilities; without, permissions empty but memberships listed.
User `{id,email,display_name,mfa_enabled,created_at}`. Tenant `{id,name,slug,status,created_at,quotas?:object}`.
GET `/auth/sessions`: Session[] `{id,created_at,last_seen_at,expires_at,current}`; DELETE `/auth/sessions/{id}`.
POST `/auth/password`: `{current_password,new_password,totp?:string}` revokes other sessions.
POST `/auth/reset/request`: `{email}` generic success regardless existence; SMTP outbox, never token in API response/logs. POST `/auth/reset/confirm`: `{token,new_password}`.
POST `/auth/mfa/enroll`: `{password}` -> `{secret,otpauth_url}` shown only while enrollment; POST `/auth/mfa/confirm`: `{totp}` -> `{recovery_codes:string[]}` once. POST `/auth/mfa/disable`: `{password,totp?:string,recovery_code?:string}`.
GET `/auth/oidc/start` redirects; GET `/auth/oidc/callback` verifies state/nonce/PKCE and issuer/subject binding.

## Tenancy and authorization controls

GET `/tenants` lists own tenants; platform admin GET `/platform/tenants` lists all; POST `/platform/tenants` `{name,slug,owner_email,quotas?}`; PATCH `/platform/tenants/{id}` `{name?,status?,quotas?}` platform only.
GET `/members`: Member[] `{id,user_id,email,display_name,department_id,position,role_ids:string[],status}`; PATCH `/members/{id}` `{department_id?,position?,role_ids?,status?}`; POST `/members/{id}/transfer` `{owner_id}` moves owned records before suspension.
GET `/invitations`: invitation metadata; POST `/invitations` `{email,department_id?,role_ids}` -> metadata (token delivered via SMTP); POST `/invitations/accept` `{token}` signed-in verified email matching invite.
GET `/departments`: Department[] `{id,name,parent_id:null|string}`; POST `/departments` `{name,parent_id?}`; PATCH `/departments/{id}` `{name?,parent_id?}`; DELETE guards referenced/nonempty/cyclic trees.
GET `/roles`: Role[] `{id,name,description,builtin,grants:Grant[]}`; POST `/roles` `{name,description,grants}`; PATCH `/roles/{id}` same; DELETE protects builtins/referenced roles; POST `/roles/{id}/copy` `{name}`.
Grant `{resource,action,scope,department_ids?:string[],read_fields?:string[],write_fields?:string[],unmask_fields?:string[],conditions?:Condition[]}`. Scope enum `self|department|department_tree|selected_departments|tenant`. Default deny; `*` fields means all nonsensitive readable/mutable fields, never arbitrary attributes. Conditions `{field,op,value}`, operators allowlisted `eq|in` (AND within grant; OR across matching grants), descriptor allowlists fields. Explicit named sensitive read field may be masked unless unmask listed.
GET `/permissions`: `{modules:Module[],scopes:string[]}` registry. GET `/members/{id}/permissions`: effective grants, authorized managers only.

## Resource modules and tickets

Module `{key,label,label_en,description,fields:Field[],actions:string[]}`. Field `{key,label,label_en,type,required?,sensitive?,readonly?,options?:string[]}` type `string|text|number|boolean|enum|date`.
Builtins registered `tickets`, `dictionaries`, `settings`, `notifications`. Control resources have permissions `members,departments,roles,invitations,audit,files,jobs` with resource-appropriate actions (read/create/update/delete/export/import/assign). Account resources never bypass tenant control authorization.
ResourceRecord `{id,tenant_id,owner_id,department_id,version,data:object,created_at,updated_at}`. Field filtering changes `data`; envelope immutable. GET `/resources/{module}` lists permitted records; GET `/resources/{module}/{id}` detail; POST `/resources/{module}` `{owner_id?,department_id?,data}`; PATCH `/{id}` `{version,owner_id?,department_id?,data}`; DELETE `/{id}` expected version via query `version=N`. Server enforces all existing/resulting scopes and fields.
Tickets fields `title` required,string; `description` text; `status` enum open/in_progress/resolved/closed; `priority` low/normal/high/urgent; `customer_email` sensitive,string; `internal_notes` sensitive,text. Initial status open. POST `/resources/tickets/{id}/transition` `{version,status}` enforces valid edges open->in_progress->resolved->closed, resolved->in_progress, closed->open. Assignment requires assign action in addition to update.
POST `/resources/{module}/export` `{columns:string[]}` -> Job; GET `/jobs/{id}/download` streams authorized CSV with current scopes/fields. POST `/resources/{module}/import` multipart CSV field `file`, max1MiB/max1000 rows, schema + scope validation, transaction all-or-nothing -> `{inserted:number}`.

## Files, jobs, audit and operations

GET `/files`: metadata list; POST `/files` multipart `file`,owner_id?,department_id?, bounded size, safe generated storage keys -> File `{id,name,size,content_type,owner_id,department_id,created_at}`; GET `/files/{id}/download`; DELETE `/files/{id}`.
GET `/jobs`: Job[] `{id,type,status,attempts,max_attempts,error,created_at,updated_at}`; GET `/jobs/{id}`; POST `/jobs/{id}/retry`; POST `/jobs/{id}/cancel`. Job statuses pending/running/succeeded/failed/cancelled. Durable leases/retries, tenant and actor revalidation. No raw errors with secrets.
GET `/audit`: Audit[] `{id,actor_id,action,resource,record_id,request_id,created_at,changes}`; no credentials/token/secret fields.
GET `/dashboard`: actual counts for permitted tenant records, recent authorized activity. No synthetic charts.
GET `/healthz`,GET `/readyz`; GET `/metrics` loopback/admin-protected or explicitly configured. Config TF_DATABASE_URL, TF_LISTEN default127.0.0.1:8090, TF_PUBLIC_URL, TF_DATA_DIR, TF_MASTER_KEY, TF_COOKIE_SECURE, TF_SMTP_HOST/PORT/FROM, TF_OIDC_ISSUER/CLIENT_ID/CLIENT_SECRET, TF_STORAGE_BACKEND local|s3 and S3 config. CLI `serve`, `migrate`, `check-config`, `version`. Developer DB local port55439; web Vite5178 proxies API/health to8090. Docker listens0.0.0.0:8090 with same-origin bundled web; Compose binds127.0.0.1:8090. SMTP development mailbox uses Mailpit.
