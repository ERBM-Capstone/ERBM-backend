# GET /api/v1/advances

Amounts are decimal strings in THB, with no currency field. Draft creation is
documented separately in [POST /api/v1/advances](advance-create.md).
Single-record reads and edits are documented in
[GET, PATCH /api/v1/advances/{id}](advance-detail.md).

## Contract

- Bearer access token required. Roles: ADMIN, ACCOUNTING, PM, PC.
- ADMIN and ACCOUNTING can read every Project; PM can read their managed
  Projects; PC can read Projects where they are the assigned PC. A Project
  has at most one PC through nullable `projects.pc_user_id`, created by
  migration 0003. Each PC may coordinate many Projects.
- Every role sees DRAFT only when `created_by` equals the authenticated user.
  This condition combines with Project access and applies before count/paging.
- Optional `project_id` must be positive. An explicit missing Project returns
  404; an inaccessible Project returns 403. Without this filter, list accessible
  Projects only.
- Optional `status`: DRAFT, PENDING_PM_APPROVAL, RETURNED_FOR_REVISION,
  APPROVED, ACTIVE, CLOSED. No additional closing statuses are introduced.
- Optional `q` searches purpose case-insensitively, at most 255 characters.
- `page`: positive integer, default 1. `page_size`: 1..100, default 20.
  The current handler also treats zero values as those defaults.
- Sort by `created_at DESC, id DESC`. Return 200 with `data: []` when empty.
- Successful envelope: `success`, `message`, `data`, `meta`. Metadata always
  contains `page`, `per_page`, and the count of visible matching records.
- Each item: `id`, `advance_code`, `requester` (`user_id`, `first_name`, `last_name`),
  `project` (`id`, `project_code`, `name`), `purpose`, `amount`,
  `status`, `created_by`, nullable `submitted_at`, `created_at`, `updated_at`.
- Errors: 400 invalid query; 401 missing/invalid authentication; 403 forbidden;
  404 explicitly selected Project missing; 500 unexpected persistence failure.
- Codes are stored at creation, e.g. `ADV-PRJ001-001`, with an independent
  sequence per Project. Requester is the creator (`created_by`), joined to their
  current employee name. The complete Advance schema is created by migration 0005.

## Implementation plan

Preserve the existing migration/document changes in the checkout. Use the
router -> handler -> service -> repository -> model/database architecture,
existing Gin/GORM/PostgreSQL dependencies, and root-level `tests/`.

1. Write failing tests for query validation, trusted actor forwarding,
   role-specific Project access, error mapping, response fields, and empty lists.
   Add the read model, DTOs, service, handler and only the GET route.
2. Write repository query tests using GORM dry runs, covering identical visible
   scope in count/select, private DRAFT for every role, Project membership,
   bound filters, stable ordering and paging. Implement the repository with
   `WithContext(ctx)` and a direct `projects.pc_user_id` membership predicate.
3. Add opt-in PostgreSQL coverage for actual visible rows/counts and pagination
   using `ERBM_MIGRATION_TEST_DATABASE_URL` and isolated test schemas. Do not
   connect the app database or load .env. Regenerate Swagger directly with the
   existing Swaggo command; do not invoke Makefile targets that load .env.
4. Run gofmt, `go test ./...`, `go vet ./...`,
   `go build ./cmd/server ./cmd/migrate`, and `git diff --check`; obtain one
   independent review before handoff. Report PostgreSQL tests as skipped when
   the explicit test database is not configured.

## Review focus

- Creator-only DRAFT must also apply to ADMIN/ACCOUNTING and status-filtered lists.
- Client-supplied actor parameters cannot replace authenticated identity.
- Count cannot include another Project or another creator's DRAFT.
- Negative pages/IDs, unsupported status and offset overflow are rejected;
  zero page/page_size retain the current handler's default behavior.
- Exact monetary text, nil submitted_at and empty-array serialization are stable.
- New tests must not depend on existing ignored test helper files.
