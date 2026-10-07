# Frontend quality upgrade implementation plan

Goal: make record authority, permission configuration and daily administration clear and reliable.

Approved specification: ../docs/superpowers/specs/2026-10-07-quality-design.md. Work is restricted to web/. Element Plus remains the component system; existing cookie/CSRF, request cancellation and dirty-field PATCH logic remain shared.

1. Record authority: first reproduce mixed read-all/update-self row actions and missing-capability denial with component tests. Add typed capability helpers; split resource controller, table, edit dialog and details. Use server transitions and registered operations. Preserve dirty edits/conflicts and department null clearing.
2. Authorization matrix: first test independent grants for the same resource/action and additive preset preview. Reuse Element Plus table and accessible buttons for selection. Move detailed scope/field/conditions into a focused grant editor. Centralize localized validation and summaries.
3. Organization boundaries: retain verified route behavior while extracting separate department, user and invitation views and focused forms/reference lookup. Record controls consume capabilities.
4. Presentation: centralize human operation/status/scope/job labels. Promote the installed Element Plus icon dependency, then unify navigation, spacing, type, tables, badges, dialogs and light/dark states. Dashboard prioritizes authorized workload without fabricated values.
5. Acceptance: preserve default-admin/security acceptance; add record-capability, matrix/preset, department-clear and light/dark/mobile scenarios. Capture dashboard, roles, resources and mobile artifacts. Run unit tests, format and build; request independent review. Commit only after coordinator approval.

Each behavior change follows failing test → implementation → passing test. Cosmetic changes are verified through browser screenshots and accessibility/layout checks rather than implementation-mirroring unit tests. Backend and coordinator own their contract and deployment work.
