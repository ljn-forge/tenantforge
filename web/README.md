# TenantForge web

Vue 3 and TypeScript administration client, using the server registry and current authorization grants. The server enforces tenant, record and field access for every operation. Authentication uses same-origin HttpOnly cookies and CSRF; the browser stores only language and theme preferences.

Requires Node 24.15 or later. TypeScript 6.0.3 is pinned because the current `vue-tsc` wrapper requires the JavaScript compiler API unavailable in TypeScript 7.

```sh
npm ci
npm test
npm run format:check
npm run build
npm run dev
```

Vite listens on `127.0.0.1:5178` and proxies the API to `127.0.0.1:8090`. Set `VITE_API_PROXY` only when using a separate development or test backend.

The supported modules come from `GET /auth/me` and `GET /permissions`. New business modules automatically receive resource lists, detail views, validated forms, operation controls and import/export interfaces from their descriptors.

Browser acceptance uses a disposable database. From the repository root, run `make browser` to create an isolated backend and Vite process, sign in with the automatically seeded administrator, change its initial password, execute Playwright and stop those processes. Do not point this command at production data. For a manually started isolated pair, use:

```sh
npm run test:e2e
```

The local browser default is installed Google Chrome. In CI or with Playwright's bundled Chromium, install it with `npx playwright install chromium`, then set `TF_E2E_CHANNEL=chromium`. `TF_E2E_URL`, `TF_E2E_USERNAME`, `TF_E2E_INITIAL_PASSWORD` and `TF_E2E_PASSWORD` can override the disposable test endpoint and account. The server seeds only an empty platform. Tests use a disposable database and change its default administrator password.

Acceptance covers default administrator login, mandatory password changes, direct user creation, department and role creation, operation scopes, ticket transitions, optimistic conflict recovery, CSV import/export and download, file upload/download, audit export, identity security events, MFA enrollment/login/recovery, session revocation, and mobile navigation/layout. MFA recovery values stay in the test process and are cleared at the end; traces and automatic screenshots are disabled.
