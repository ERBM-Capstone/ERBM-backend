# POST /api/v1/advances

Creates a DRAFT Advance. Each Project has at most one assigned PC; only that
authenticated PC can create it. A closed Project returns 409.

```json
{
  "project_id": 10,
  "purpose": "ค่าเดินทางและที่พัก",
  "amount": "15000.00"
}
```

All three fields are required. Project ID must be positive and purpose must
remain nonempty after trimming. Amount is a positive decimal string in THB,
with at most 13 integer digits and 2 decimals. Zero, negative, missing,
non-string, exponent notation, excess precision and overflow return 400.
The server normalizes accepted amounts to two decimals without floating point.

The server assigns DRAFT, creator/updater from authentication, and an immutable
`advance_code`: `ADV-{project_code}-{per-project number}`. Examples:
`ADV-PRJ001-001`, `ADV-PRJ001-002`, `ADV-PRJ002-001`. Padding is a minimum of
three digits; numbers beyond 999 expand normally. The Project code is used as
stored, including any hyphens. Clients cannot choose the code, actor or status.

An existing ACTIVE Advance does not block creating another DRAFT. At most one
Advance can be ACTIVE per Project, enforced by migration 0005's partial unique
index. This endpoint does not activate or submit an Advance. Drafts remain
visible only to their creators through the list endpoint.

Creation locks the Project row to serialize with Project closing/assignment
updates, checks that `projects.pc_user_id` matches the authenticated PC and checks
Project status, allocates a number
with an atomic per-Project counter UPSERT, and saves both Advance and CREATE
event in one transaction. Failure rolls back the number, record and audit.
The event stores actor, previous status null, resulting DRAFT, timestamp and
an after snapshot with exact monetary text.

The user confirmed workflow history with request snapshots for each round,
including historical purpose and amount, rather than only a timeline of actions.
Later edits must preserve earlier snapshots. The CREATE event provides the
initial snapshot; submission/resubmission history and revision retrieval belong
to their upcoming endpoints and are not implemented by this endpoint.

Returns 201 with the standard `success`, `message: "Created"`, `data` envelope.
Data has the same shape as a [GET list item](advance-list.md), including
`id`, `advance_code`, requester identity/name, Project, purpose, amount, DRAFT,
creator, null `submitted_at`, creation time and update time. There is no currency
field or list metadata. Error statuses: 400 invalid body, 401 unauthenticated,
403 role/membership denied, 404 Project missing, 409 Project closed, 500
unexpected persistence failure without database details.

## Deployment and migration

The user reports that only versions 0001, 0002 and 0004 are applied locally.
Versions 0003 and 0005 are confirmed unapplied, so corrections are folded into
those creation migrations directly at the user's request. No additional delta
migration is required for these changes. Database state was not inspected.

Migration 0003 creates nullable `projects.pc_user_id` as a foreign key to users
with `ON DELETE RESTRICT` and a nonunique index. A Project has at most one PC,
and that PC may coordinate many Projects. Migration 0005 creates
Advances with required unique codes, workflow events and per-Project counters
from the start. Counters start on first allocation; no existing Advance backfill
is needed. The runner applies pending 0003 before 0005 and skips the already
applied versions. Existing API behavior and transaction protocol are unchanged.

Down for 0005 drops Advances, events and counters, losing their data. Project
schema is preserved. If rolling back 0003 as well, its dependent Advance schema
must be removed first. Back up needed data before a destructive rollback.

## Verification

Root-level tests cover input/money, actor trust, membership, Project status,
response fields and audit errors. PostgreSQL tests cover creation-time constraints and codes beyond 999,
up/down/up, per-Project sequences, concurrent creations beside an ACTIVE,
audit-failure rollback, stable stored codes and the existing activation guard.
They run only with an explicitly configured disposable PostgreSQL database in
`ERBM_MIGRATION_TEST_DATABASE_URL`, each in an isolated schema. They never load
application config or .env, and skip when that variable is absent.
