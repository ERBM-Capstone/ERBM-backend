# Project coordinator

Confirmed rule: each Project has exactly one PM and at most one PC. Each PM
may manage many Projects, and each PC may coordinate many Projects. Only the
assigned PC may create Advance Requests. The role PC alone does not grant
access to other Projects, so backend membership checks remain in place.

## API contracts

POST `/api/v1/projects` generates `project_code` on the server and accepts a
single optional `pc_user_id`:

```json
{
  "name": "Project A",
  "budget": "1000.00",
  "pm_user_id": 100,
  "pc_user_id": 25
}
```

The client omits `project_code`. A supplied value is ignored. Responses retain
`project_code`, for example `PRJ-2026-000001`. The Gregorian year comes from the
creation timestamp in Asia/Bangkok (UTC+7), and each year starts at 000001.
The number has a minimum width of six digits and is never truncated. Codes are
assigned once and cannot be changed through PATCH.

`project_code_counters` stores one counter per year. Allocation uses an atomic
UPSERT, and the counter and Project insert share a transaction. Concurrent
requests for the same year serialize on that counter row; a failed transaction
rolls back both its counter allocation and its Project. Member eligibility
queries use the same transaction connection. Existing Project codes remain as
stored, including codes used in historical Advance identifiers.

- ADMIN/PM may omit `pc_user_id` or send null to create an unassigned Project.
- A positive integer `pc_user_id` assigns that PC, subject to role/account checks.
  Arrays, strings, fractional and nonpositive IDs return 400.
- A PC creating a Project is assigned automatically. They may omit `pc_user_id`
  or send their own ID; sending another PC returns 400.
- The same PC may be assigned to multiple Projects.

PATCH `/api/v1/projects/{id}` uses the same scalar `pc_user_id` field:

| Body | Result |
| --- | --- |
| `{ "name": "Project A updated" }` | Preserve the assigned PC. |
| `{ "pc_user_id": null }` | Remove the assigned PC. |
| `{ "pc_user_id": 26 }` | Assign or replace the PC with user 26. |

Only ADMIN or that Project's PM may change the PC assignment. An assigned PC
may edit only the Project name and description. Assignment targets must be
ACTIVE users with the PC role; the required PM must likewise be an ACTIVE
user with the PM role. A Project without an assigned PC has no PC authorized
to create an Advance.
Arrays, strings, zero, negative and fractional IDs return 400.

PC field presence follows the same case-insensitive matching as JSON decoding.
For example, `PC_USER_ID: null` removes the PC just like `pc_user_id: null`;
the documented field name remains `pc_user_id`. Presence is tracked separately
from its nullable value, so omitting the field always preserves the assignment.

PATCH checks Project and field permissions before checking CLOSED status.
An actor denied permission receives 403 whether the Project is ACTIVE or CLOSED;
an otherwise permitted edit of a CLOSED Project receives 409.

Only the current Project PM may close the Project with `status: "CLOSED"`.
Closing succeeds when it has no Advances or every Advance is `CLOSED`.
Any other Advance status, including DRAFT, returns 409 with message
`project has advances that are not CLOSED`. No field in that PATCH is saved.
A closed Project cannot be edited or reopened. The check holds the same
Project row lock as Advance creation, so concurrent creation and closing cannot
leave a closed Project with a newly created DRAFT. Assignment eligibility
queries share the update transaction's connection; PC permissions use the
locked Project's `pc_user_id`.

Project responses expose `pc` as one member object or `null`, with the same
member fields as `pm`:

```json
{
  "pm": {
    "user_id": 100,
    "first_name": "Pat",
    "last_name": "Manager",
    "email": "pat@example.com"
  },
  "pc": {
    "user_id": 26,
    "first_name": "Casey",
    "last_name": "Coordinator",
    "email": "casey@example.com"
  }
}
```

Clients must use `pc_user_id` for both POST and PATCH, and read `pc` from the
response. The former `pc_user_ids` request field and `pcs` response array are
removed. In Go, `Project.PCUserID` is `*int64` and `ProjectDetail.PC` is
`*ProjectUserSummary`.

The service gathers the distinct `pm_user_id` and non-null `pc_user_id` values
from the Projects it already loaded. `GetMembersByUserIDs` loads their user
profiles in one query, and each Project is assembled using those same User IDs.
The profile contains `UserID`, `FirstName`, `LastName`, and `Email`; it has no
Project ID because the loaded Project supplies its own assignments. A subsequent
reassignment cannot substitute a different member in the response being built.
These summaries use current user/employee profile data, rather than a historical
snapshot of names or email addresses.

## Migration 0003

The user confirms 0003 has not been applied and asks to fold corrections into
unapplied creation migrations. It creates nullable `projects.pc_user_id BIGINT`
as a foreign key to `users(id)` with `ON DELETE RESTRICT` and a nonunique
`idx_projects_pc_user_id` index. The required `pm_user_id` remains a nonunique
foreign key to users. The scalar PC column allows at most one PC per Project,
while its nonunique index allows the same PC on many Projects.

There is no coordinator join table. No legacy assignment backfill is needed
because 0003 is confirmed unapplied. The application's database was not accessed.

The same unapplied migration also creates `project_code_counters` with a `year`
INTEGER primary key and a positive `last_number` BIGINT. Its down migration
removes the counter table together with Projects. Code formatting is performed
by the service; SQL inserts outside the service must still provide project_code.

Down for 0003 drops the Project table and its data. Dependent Advance
schema must be removed first. The PostgreSQL tests perform these operations only
inside disposable test schemas, and check that the schema is recreated on up.

## Verification

Root-level tests cover scalar POST assignment, invalid scalar values/types,
automatic assignment, PATCH omission/null/replacement, response object/null,
authorization and direct PC membership. Opt-in PostgreSQL tests cover direct
FK/index shape, shared PC assignment across Projects, invalid user references,
assignment replacement/removal, up/down/up, and Advance creation before/after
reassignment. They use the explicit
disposable database `ERBM_MIGRATION_TEST_DATABASE_URL` and isolated schemas;
without that configuration they skip, without reading .env or application DB
configuration.

`project_review_test.go` covers authenticated reads, membership scope, complete
PATCH validation, closing with each Advance status, lookup failures and paging
overflow. `project_review_postgres_test.go` additionally checks updates with one
database connection, rollback on blocked closing, and concurrent Advance
creation/Project closing in both lock orders. These PostgreSQL tests use the
same opt-in disposable database and skip when it is not configured.

`project_consistency_test.go` covers member identity during reassignment,
profile lookup failures, permission checks on CLOSED Projects, case-insensitive
PC null/ID handling, and nil request guards at the service boundary. A nil
Create/Update request returns BadRequest instead of panicking.

`project_code_test.go` covers HTTP creation without a code, ignoring client
codes, sequential server-generated responses, allocation failures and rejection
before allocation. `project_code_postgres_test.go` uses the same disposable
database setup to cover independent annual counters, rollback, concurrent
Project creation and Advance identifiers built from generated Project codes.
