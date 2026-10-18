# GET, PATCH /api/v1/advances/{id}

Both endpoints require a Bearer access token and a positive int64 Advance ID.
They return 200 with the standard `success`, `message: "OK"`, `data` envelope.
Data has the same fields as an [Advance list item](advance-list.md), including
the stored immutable code, requester, Project, exact THB amount and timestamps.
There is no list metadata. A missing Advance returns 404.

## GET

ADMIN and ACCOUNTING can read every Project. PM must be its assigned manager;
PC must be its current assigned coordinator. Every role can read a DRAFT only
when they are its creator. Denied access returns 403. Closed Advances can be
read with the same access rules.
Current data, status and Project membership are read in a single SQL statement
so authorization cannot combine older data with a newer assignment.

## PATCH

Only the PC creator who is still assigned to the Project can edit the Advance.
It must be DRAFT or RETURNED_FOR_REVISION; other statuses return 409.
A CLOSED Project also returns 409. A successful edit results in DRAFT.

```json
{
  "purpose": "ค่าเดินทางและที่พัก",
  "amount": "12000.00"
}
```

Both fields are optional individually. Omitted or null fields keep their
current values. At least one non-null supported field is required; an empty
patch returns 400. Purpose must remain nonempty after trimming. Amount uses
the same positive NUMERIC(15,2) decimal-string rules as
[creation](advance-create.md): at most 13 integer digits and two decimals,
normalized without floating point. Numeric JSON values, zero, negatives,
exponents, excess precision and overflow return 400.

Unrecognized fields are ignored, following existing request binding. Clients
cannot change Project, creator/updater identity, code, submission metadata or
choose a status. Identity always comes from authentication.

The service locks Project then Advance, and rechecks the current assignment,
creator and status under those locks. Partial edits merge into the locked
record so simultaneous edits to different fields preserve both changes.
The mutation and an append-only UPDATE event commit in one transaction. The
event records actor, previous/resulting status, timestamp, and before/after
snapshots with exact money strings. Earlier CREATE and UPDATE snapshots remain
unchanged. A persistence/audit/response-read failure rolls the transaction back
and returns a generic 500.

UPDATE events are audit history; this endpoint does not assign submission
revision numbers. Existing submission timestamps are retained when a returned
request becomes a draft again.

Errors: 400 invalid ID/body, 401 unauthenticated, 403 role/ownership/Project
access denied, 404 Advance missing, 409 noneditable status/closed Project,
500 unexpected persistence failure without database details.

## Verification

Root-level tests cover routes, trusted actor, Project access, private drafts,
partial edits, exact money, immutable fields, status transitions, snapshot
contents, error mapping, lock queries and bound updates. PostgreSQL tests
add concurrent partial updates, historical snapshot preservation and rollback
after an audit failure. They use the existing isolated disposable-database
helper and skip unless `ERBM_MIGRATION_TEST_DATABASE_URL` is configured.
They never load application config or `.env`.

### Checks run on 2026-10-18

Passed (PostgreSQL cases skip without the explicit test database URL):

```sh
go test tests/advance*.go -run 'TestAdvance(Detail|Get|Patch|List|Repository)' -count=1
go vet ./cmd/... ./config/... ./dto/... ./internal/... ./docs/...
go build ./cmd/server ./cmd/migrate
git diff --check
```

Changed Go files were formatted with `gofmt -w`; `gofmt -l` on all changed Go
files returned no paths. Swagger generation succeeded using:

```sh
go run github.com/swaggo/swag/cmd/swag init -g cmd/server/main.go -o docs
```

Swaggo emitted its nonfatal warning that the repository root has no Go files.
An independent read-only review found no remaining actionable issues after
the single-snapshot GET correction.

Full checks were attempted and retain pre-existing checkout failures:

```sh
go test ./...
go vet ./...
```

Both stop on mixed packages: the user's moved, ignored `tests/project_test.go`
still declares `package service`, while the root test suite uses `package tests`.
The separate baseline check below also fails its `anonymous` case because the
existing local edit removed the actor guard from `AdvanceService.Create`:

```sh
go test tests/advance*.go -run TestAdvanceCreateEnforcesActorProjectAndAtomicAudit -count=1
```

Those prior edits were preserved. PostgreSQL concurrency/rollback tests were
added but not executed against a database in this session.
