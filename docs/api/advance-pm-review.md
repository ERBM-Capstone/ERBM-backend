# PM approval and return

Both endpoints require a Bearer access token, the PM role, and a positive
int64 Advance ID. Only the Project's currently assigned PM may review its
requests. ADMIN does not override this permission.

## Approve

```http
POST /api/v1/advances/7/approve
Authorization: Bearer <PM_access_token>
```

No body is required. The transition is
`PENDING_PM_APPROVAL -> APPROVED`. The server records `approved_at`,
`approved_by`, `updated_at` and `updated_by` from its clock and authenticated
actor. Approval does not activate the Advance or confirm disbursement.

## Return for revision

```http
POST /api/v1/advances/7/return
Authorization: Bearer <PM_access_token>
Content-Type: application/json

{
  "reason": "กรุณาชี้แจงรายละเอียดค่าเดินทาง"
}
```

`reason` is required and must be a JSON string. Leading/trailing whitespace
is trimmed; an empty or whitespace-only reason returns 400. No additional
length rule is imposed. The transition is
`PENDING_PM_APPROVAL -> RETURNED_FOR_REVISION`. The reason is saved in the
RETURN audit event. PC editing and resubmission continue through the existing
PATCH and submit endpoints.

## Responses

Both endpoints return 200 with the existing Advance detail DTO and the standard
envelope. For example, after approval:

```json
{
  "success": true,
  "message": "OK",
  "data": {
    "id": 7,
    "advance_code": "ADV-PRJ-2026-000001-001",
    "requester": {
      "user_id": 25,
      "first_name": "Somchai",
      "last_name": "Jaidee"
    },
    "project": {
      "id": 10,
      "project_code": "PRJ-2026-000001",
      "name": "Project A"
    },
    "purpose": "Travel",
    "amount": "15000.00",
    "status": "APPROVED",
    "created_by": 25,
    "submitted_at": "2026-10-31T09:00:00+07:00",
    "created_at": "2026-10-31T08:30:00+07:00",
    "updated_at": "2026-10-31T10:00:00+07:00"
  }
}
```

A returned request has the same fields, with status
`RETURNED_FOR_REVISION`. Money stays an exact THB decimal string. These
responses have no pagination metadata. Approval metadata and the return reason
are stored internally; this change preserves the existing list/detail DTOs.
Read action history and return reasons through
[GET /api/v1/advances/{id}/history](advance-history.md).

## Validation and atomicity

- Only pending requests in an open Project can be reviewed. Repeating an
  action or trying the other action after success returns 409.
- Actor IDs come from authentication. Body/query fields cannot change the
  actor, amount, purpose, Project, code, creator or submission metadata.
- The service reads the immutable Project ID, starts a transaction, locks
  Project then Advance, and rechecks current PM assignment and statuses.
- Status/metadata and the APPROVE or RETURN event commit together. Events
  include actor, previous/resulting status, time, before/after snapshots, and
  the required reason for RETURN. Response-read and commit failures also
  roll back the action.
- Concurrent approve/return requests serialize on these locks. Only the first
  valid review commits; later reviews see the changed status and return 409.
- Review events do not allocate submission revisions or change old snapshots.

Existing migration 0005 already defines approval columns and the event reason.
No schema changes are needed and no application migrations are run.

| Status | Meaning |
| --- | --- |
| 400 | Invalid ID, malformed return JSON, missing or blank reason. |
| 401 | Missing/invalid authentication or nonpositive service actor ID. |
| 403 | Wrong role or PM is not currently assigned to the Project. |
| 404 | Advance does not exist. |
| 409 | Advance is not PENDING_PM_APPROVAL, or its Project is closed. |
| 500 | Unexpected persistence failure, without database details. |

## Tests

`tests/advance_pm_review*_test.go` cover trusted actor/assignment, both
transitions, exact money, immutable request/submission contents, audit reasons
and snapshots, failure rollback, duplicate/conflicting actions, HTTP validation,
real route role gates, Swagger, and bound repository writes.

PostgreSQL tests use isolated schemas only when
`ERBM_MIGRATION_TEST_DATABASE_URL` explicitly names a disposable test database.
They cover concurrent approve/return, persisted metadata/reasons, unchanged
submission revision counts, and rollback when audit insertion fails. With no
explicit test database, those cases skip; application configuration is unused.

## Verification on 2026-10-31

Passed:

```sh
go test tests/advance_pm_review*_test.go tests/service_actor_guard_test.go -count=1 -v
go test tests/advance_pm_review*_test.go tests/service_actor_guard_test.go tests/advance_worklist_test.go tests/advance_create_test.go tests/advance_detail_test.go tests/advance_list_test.go tests/advance_repository_test.go tests/advance_submit_repository_test.go tests/project_contract_test.go tests/swagger_test.go -count=1
go vet ./cmd/... ./config/... ./dto/... ./internal/... ./docs/...
go build ./cmd/server ./cmd/migrate
go run github.com/swaggo/swag/cmd/swag init -g cmd/server/main.go -o docs
git diff --check
```

All changed Go sources were formatted with `gofmt -w`. Swagger and Postman
JSON were parsed, both POST paths were verified, and the required `reason`
schema and Postman return body were checked. Swaggo emitted its existing
nonfatal warning about no Go files in the repository root.

`go test ./...` and `go vet ./...` were attempted and still fail on existing
local tests: `tests/advance_submit_test.go:301` refers to removed
`SubmittedBy`/`SubmittedAt` DTO fields, while `tests/project_code_test.go:78,80`
refers to a removed `CreatedAt` field. These unrelated contracts/tests were
preserved.

PostgreSQL cases skipped because `ERBM_MIGRATION_TEST_DATABASE_URL` was not
configured. Actual database concurrency and rollback therefore remain
unverified in this session. The unit transaction fixture and repository
DryRun checks passed; no application migrations or containers were run.
