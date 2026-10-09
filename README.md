# TenantForge

[简体中文](README.zh-CN.md)

[![CI](https://github.com/ljn-forge/tenantforge/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/ljn-forge/tenantforge/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-teal.svg)](LICENSE)

A Go/Vue foundation for multi-tenant enterprise applications, with a complete internal customer-support reference workflow.

Reuse authentication, organizations, roles, tenant isolation and record/field authorization while building your business modules. The support example connects issue registration, supervisor assignment, agent queues, replies/private notes, resolution summaries, closure and reopening.

[Walk through a ticket from administrator setup to agent resolution](docs/support-desk.md#english-overview).

TenantForge combines a same-origin web console with a Go service and PostgreSQL. Configure permissions per action: a tenant-wide read grant never expands a self-only update grant. The runtime database account cannot own protected tables or bypass PostgreSQL row-level security.

[Quick start](#start-locally) · [Feature tour](#feature-tour) · [Documentation](#documentation) · [Verification](#verify)

## What can you build?

Use TenantForge for an internal administration system or a SaaS console serving
multiple companies. Each company has its own employees, departments, roles and
business records. Build the domain module while reusing the platform services.

| Need                            | Included solution                                                                        |
| ------------------------------- | ---------------------------------------------------------------------------------------- |
| Get employees into the system   | Default superadministrator, direct account creation, departments and roles               |
| Serve several companies         | Tenant provisioning, membership, switching, quotas and forced database RLS               |
| Control who can do what         | Independent action grants, record scopes and field read/write/unmask access              |
| See a complete business example | Registrar → supervisor assignment → agent handling → resolution → closure/reopening      |
| Add your own business           | Public Go resource/operation/job registration; asset approval and inventory example      |
| Run and maintain it             | Same-origin container deployment, migrations, protected files, audit and backup/recovery |

## Start locally

Requirements: Docker with Compose, Python 3.12 or newer. Development also requires Go 1.26.9 or a supported newer patched release, and Node 24.21 or a supported newer LTS release.

```sh
git clone https://github.com/ljn-forge/tenantforge.git
cd tenantforge
python3 scripts/init-env.py
docker compose --profile app up -d --build --wait
```

Open http://localhost:8090 and sign in as `admin` with the initial password `Admin@123456`. Change the initial password when prompted, then use User management to create accounts and assign departments/roles. A default tenant is provisioned automatically. Configuration and encryption keys are generated locally in the ignored, private `.env` file. Initial credentials are configurable and never replace an existing administrator.

Development:

```sh
make init
make deps
make dev
```

Console: http://localhost:5178. Local email mailbox: http://localhost:8029. The database is exposed only on loopback port 55439.

## Included

- Workspace provisioning, suspension, quotas, membership, invitation email, tenant switching, departments and positions.
- Resource/action permission matrix, additive presets and grant preview, role copying and effective permission inspection; self, department, department tree, selected departments and tenant scopes.
- Server-authoritative per-record capabilities, action-specific predicates, typed conditions, masked sensitive fields and guarded edits, assignments and exports.
- Actual-workload dashboard, Chinese/English presentation, light/dark themes and responsive layouts.
- Default superadministrator, direct user creation, opaque cookie sessions, CSRF, password recovery, session revocation, TOTP and recovery codes; optional configured OIDC.
- Support-agent/supervisor role templates, personal queues, independent assignment authority, scoped activity/replies/private notes, required resolution summaries, closure/reopening and optimistic versions; dictionaries, settings, notifications, CSV import/export, protected files, durable jobs and audit records.
- Public Go extension interfaces for registered record operations and background jobs, including permission-filtered task discovery, generated input forms and bilingual typed results; asset approval/inventory is a complete example.
- Short upload transactions and quota reservations, independent job/mail/collection lanes, typed diagnostics, Prometheus metrics, migration/runtime separation, backup/recovery and container deployment.

## Feature tour

Every screenshot below comes from a persisted browser-acceptance fixture.
Fresh installations create the administrator and default tenant; the example
employees, tickets and assets shown here are not automatically seeded.

### Administrator login and account setup

The application starts with an administrator login. The initial administrator
changes their password, creates employee accounts, and assigns departments and
roles from the console. There is no workspace-creation form before sign-in.
Account security also provides session revocation, TOTP and recovery codes.

![Administrator login with username/email and password](docs/assets/login.png)

### Workload dashboard and tenant switching

Start with open/urgent tickets, running/failed jobs and team users. Cards link to
the corresponding pages; recent activity leads into audit. Counts reflect the
signed-in user's authorized records. The organization selector changes the
current tenant, and navigation follows available permissions.

![English workload dashboard, recent activity and quick access](docs/assets/dashboard.png)

<details>
<summary>Chinese dashboard and desktop dark theme</summary>

The Chinese dashboard exposes the same workload cards and business shortcuts.

![Chinese workload dashboard](docs/assets/dashboard-zh.png)

The desktop dark theme keeps status colors, activity and navigation readable.

![Desktop dashboard in dark mode](docs/assets/dashboard-dark.png)

</details>

### Roles and independent operation permissions

Choose a resource and operation in the permission matrix, then configure that
grant's scope and fields. Reading all tenant tickets does not give authority to
edit them all. Multiple grants for one operation remain independent; presets
add missing grants and can be reviewed before saving. Support-agent and
supervisor templates open new role drafts for administrator review.

![Role permission matrix with independent resource/action grants](docs/assets/roles-matrix.png)

<details>
<summary>Chinese role permission matrix</summary>

The localized role editor shows granted and ungranted operations separately.

![Chinese resource/action permission matrix](docs/assets/roles-zh.png)

</details>

### Record scopes and sensitive fields

Limit an operation to own records, a department, its descendants, selected
departments or the tenant. Add typed conditions when needed. Field access
separates reading, writing and explicitly unmasking sensitive values such as
customer email and internal notes. Authorization runs on the server for
queries, counts, details, writes, exports and jobs.

![Own-record update scope with field read, write and unmask controls](docs/assets/roles-grant.png)

<details>
<summary>Chinese scope and field editor</summary>

This grant allows editing owned records while leaving sensitive fields
ungranted. The server also validates the administrator's delegation authority.

![Chinese record-scope and field-permission editor](docs/assets/roles-grant-zh.png)

</details>

### Business pages and per-record actions

Search, filter and paginate records through the shared business page. Actions
come from the server's capabilities for each record. In this example, the
employee can view both tickets but edit only their own ticket.

![Employee can edit an owned ticket and only view a team ticket](docs/assets/resource-authority.png)

The detail drawer displays authorized fields and available state transitions.
Restricted fields are omitted or masked according to the current grants;
technical identifiers stay in a collapsible section.

![Chinese record drawer with filtered fields and a ticket transition](docs/assets/resource-zh.png)

### Complete internal support workflow

An administrator creates the support department, reviews the agent/supervisor
role templates, and adds employees. An agent registers a customer's problem;
the supervisor assigns another agent. The new owner finds it in My active
tickets, starts work, records communication and private notes, enters a required
resolution summary, and closes it. Reopening keeps the handling history.

The personal queue contains owned open/in-progress tickets. Other queues show
authorized in-progress, resolved, closed or all tickets, with search and
priority/status filters. Reassignment removes the former owner's self-scope
access.

![Agent's personal queue with customer issue, priority and resolution](docs/assets/support-queue-zh.png)

Activity records registration, assignment, state changes, communication and
private notes. Every history read checks the current parent-ticket and field
permissions. Private notes are excluded from both results and totals for
employees without the necessary access. Expected versions protect concurrent
changes; closed tickets cannot accept more replies.

![Ticket activity with assignment, customer communication, private note and resolution](docs/assets/support-ticket-zh.png)

The same activity drawer supports mobile handling and a dark theme; sensitive
notes remain visually distinct and subject to the same server permissions.

<img src="docs/assets/support-mobile-dark.png" alt="Mobile dark ticket activity with private notes and state changes" width="390">

Replies store an employee's communication record. This example does not send
customer email or provide a customer self-service portal.
[Follow the complete support tutorial](docs/support-desk.md#english-overview).

### Public extensions and background tasks

The opt-in asset example registers a business module, approval operation and
inventory task through public Go packages. The task catalog shows only task
types authorized for the current employee; declared inputs generate the form.
This inventory task needs no extra input and counts visible assets.

![Generated asset inventory task creation form](docs/assets/extension-task-create-zh.png)

After execution, the drawer presents typed results such as Visible asset count,
status and attempts. Durable jobs support retry/cancellation and revalidate
membership, operation and record access during execution.

![Completed asset inventory job with its typed count result](docs/assets/extension-task-zh.png)

The generated task form also works on a narrow screen in dark mode.

<img src="docs/assets/extension-task-mobile-dark.png" alt="Generated asset inventory task form on mobile in dark mode" width="390">

Assets are not enabled in the stock binary. Follow the
[extension guide](docs/extensions.md) to run the public example or register your
own business modules without changing core routes or authorization.

### Mobile navigation and light/dark themes

On a narrow screen, the sidebar becomes a navigation menu and workload cards
reflow. Both screenshots show the same authorized dashboard data.

| Mobile light theme                                                                                    | Mobile dark theme                                                                                   |
| ----------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| <img src="docs/assets/mobile-light.png" alt="Mobile light dashboard with workload cards" width="390"> | <img src="docs/assets/mobile-dark.png" alt="Mobile dark dashboard with workload cards" width="390"> |

## Documentation

- [Support workflow and developer reuse](docs/support-desk.md)
- [Installation and configuration](docs/installation.md)
- [Permission semantics and security](docs/security.md)
- [REST contract](docs/api-contract.md)
- [OpenAPI 3.1 specification](docs/openapi.json)
- [Operations, backup and upgrades](docs/operations.md)
- [Module extensions](docs/extensions.md)
- [Dependency decisions](docs/dependencies.md)
- [Verification evidence](docs/verification.md)

## Verify

```sh
make check
make integration
python3 -m unittest discover -s tests
cd web && npm audit --omit=dev
```

Database integration tests require the isolated `tenantforge_test` database generated by local initialization. They refuse to reset another database. Browser acceptance tests use a separate database; see the verification guide.

MIT licensed. See [LICENSE](LICENSE).
