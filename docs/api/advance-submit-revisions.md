# Submit and submitted revisions

Both endpoints require a Bearer access token and a positive int64 Advance ID.
They return 200 with the standard `success`, `message: "OK"`, `data` envelope,
without pagination metadata. Missing Advances return 404.

## POST /api/v1/advances/{id}/submit

No request body is required. The authenticated PC must be the creator and the
current assigned PC of the Project. The Project must not be CLOSED.
Only DRAFT can transition to PENDING_PM_APPROVAL. A request in
RETURNED_FOR_REVISION must first be edited through PATCH to become DRAFT;
repeated submission and every other status return 409.

Submission uses the stored purpose and exact positive THB amount. Client body
or query fields cannot change the amount, status, code, Project, or actor.
The response has the same fields as [GET detail](advance-detail.md), with status
PENDING_PM_APPROVAL and the latest `submitted_at`.

The service locks Project then Advance, rechecks ownership, current assignment,
Project status and request status, then writes `submitted_at`, `submitted_by`,
`updated_at`, `updated_by` and status. The SUBMIT event records the authenticated
actor, previous/resulting status, timestamp and before/after snapshots in the
same transaction. Failure to persist the submission, event, response read or
commit rolls everything back. Concurrent duplicate submissions produce one
successful transition and one revision; the others return 409.

The first successful submission is revision 1. Each successful resubmission adds
one revision. Existing snapshots are append-only; CREATE and UPDATE audit
events remain stored but do not count as submitted revisions.

Errors: 400 invalid ID or incomplete/invalid stored request, 401 unauthenticated,
403 role/creator/assignment denied, 404 Advance missing, 409 non-DRAFT or closed
Project, 500 unexpected persistence error without database details.

## GET /api/v1/advances/{id}/revisions

Returns only successful SUBMIT snapshots, oldest submission first, with
`revision` starting at 1 **per Advance**. Event IDs determine stable submission
order even when timestamps tie. No revision number is allocated by saving a
Draft or by a failed/duplicate submission. A never-submitted Advance returns
`data: []`.

```json
{
  "success": true,
  "message": "OK",
  "data": [
    {
      "revision": 1,
      "submitted_by": 25,
      "submitted_at": "2026-10-19T02:00:00Z",
      "snapshot": {
        "id": 7,
        "advance_code": "ADV-PRJ001-001",
        "project_id": 10,
        "purpose": "ค่าเดินทางและที่พัก",
        "amount": "15000.00",
        "status": "PENDING_PM_APPROVAL",
        "created_by": 25,
        "updated_by": 25,
        "created_at": "2026-10-18T02:00:00Z",
        "updated_at": "2026-10-19T02:00:00Z"
      }
    }
  ]
}
```

Snapshot fields come from the submitted event, never from the current request.
Historical purpose, amount and code therefore survive later edits. Monetary
values remain decimal strings. `submitted_at` and `submitted_by` identify the
actor and time of that specific round.

Access follows existing GET detail rules: ADMIN and ACCOUNTING may read all
Projects; PM and PC must be currently assigned to the Project. A current DRAFT
is private to its creator for every role, including its earlier submitted
history while being revised. Access is based on current status and assignment,
read in the same SQL statement as the snapshots. A missing Advance is distinct
from an existing Advance with no submitted revisions.

Errors: 400 invalid ID, 401 unauthenticated, 403 access denied, 404 Advance
missing, 500 unexpected history-read or snapshot-decoding failure.

## Storage and verification

Uses the existing submission columns and `advance_workflow_events` in migration
0005. No schema migration is needed or run.

Root-level tests cover route registration, trusted actor, ownership/current
assignment, private drafts, invalid states, submission snapshots, per-Advance
revision numbering, exact money, empty history and error responses. Repository
tests check bound SQL, context and the single-statement visibility/history read.
Optional PostgreSQL tests cover concurrent duplicate submissions, audit-failure
rollback, metadata persistence, actual snapshot reads and resubmission history.
These use isolated schemas only when `ERBM_MIGRATION_TEST_DATABASE_URL` points
to an explicitly configured disposable database; they otherwise skip.

### Checks run on 2026-10-19

Passed:

```sh
go test tests/advance*.go -run 'TestAdvance(Submit|Submission|Revisions|Detail|Get|Patch)' -count=1
go test tests/advance*.go -run 'TestAdvance(Submit|Submission|Revisions|Detail|Get|Patch|List|Repository)' -count=1 -v
go vet ./cmd/... ./config/... ./dto/... ./internal/... ./docs/...
go build ./cmd/server ./cmd/migrate
go run github.com/swaggo/swag/cmd/swag init -g cmd/server/main.go -o docs
git diff --check
```

All changed Go files were formatted with `gofmt -w`; `gofmt -l` returned no
paths. Swagger emitted its existing nonfatal warning about no Go files in the
repository root. PostgreSQL cases skipped because the explicit disposable test
database URL was not configured, so live database concurrency and rollback
remain unverified in this session.

Full checks were attempted and still fail on existing checkout issues:

```sh
go test ./...
go vet ./...
go test tests/advance*.go -count=1
```

The first two fail on the mixed packages in the ignored local
`tests/project_test.go` (`service` versus `tests`). The full Advance suite fails
only `TestAdvanceCreateEnforcesActorProjectAndAtomicAudit/anonymous`, because
the existing Create service lacks its positive-actor guard. Neither issue was
changed by the Submit/Revisions implementation.
