# Accounting confirms disbursement

```http
POST /api/v1/advances/7/disburse
Authorization: Bearer <ACCOUNTING_access_token>
```

No request body is required. Accounting confirms that the full approved amount
has been paid once. The server copies the exact stored Advance amount; callers
cannot override the amount, actor, status or payment time through body/query fields.
The transition is `APPROVED -> ACTIVE`. Partial payments and multiple payments
are outside this confirmed flow. MD paper signature remains outside the system
in phase 1; no signed-document upload is introduced.

## Client steps

1. PM approves using `POST /api/v1/advances/{id}/approve` (no body).
2. Accounting lists `GET /api/v1/advances?status=APPROVED` and reads
   `GET /api/v1/advances/{id}` before making payment.
3. After full payment, Accounting calls `POST /api/v1/advances/{id}/disburse`.
4. The `200` response uses the existing `success`, `message: "OK"`, `data`
   envelope with the Advance detail DTO, unchanged `amount` and `status: "ACTIVE"`.
5. PC can record expenses while the Advance is ACTIVE. Read the `DISBURSE` action
   through `GET /api/v1/advances/{id}/history`.

Only ACCOUNTING may confirm disbursement; ADMIN, PM and PC do not override this
permission. Accounting has access across Projects, consistent with its existing
Advance list/detail permissions. The Project must not be CLOSED. One Project
may have only one ACTIVE Advance; other pending/approved requests are unchanged.

## Persistence and concurrency

The service locks Project then Advance, rechecks statuses, and checks for another
ACTIVE Advance while holding the Project lock. It writes one
`advance_disbursements` row, `advances.status`, `activated_at`, `updated_at/by`
and a `DISBURSE` workflow event in a single transaction. Request, submission and
PM approval fields are preserved. Payment, confirmation, activation and event
timestamps come from the same server clock reading. `disbursed_at` records the
confirmation time; it is not a user-entered historical bank-transfer timestamp.

The payment stores exact `NUMERIC(15,2)` THB, with `confirmed_by` from authentication.
The audit event stores actor, before/after snapshots, status transition and time.
History returns this event without allocating another submission revision;
`revision`, `amount` and `purpose` remain null for non-SUBMIT events as before.

Unique constraints enforce one payment per Advance and one ACTIVE Advance per
Project, including concurrent requests. Repeated confirmation returns `409` and
does not create another payment or event. Any write, audit, response-read or
commit failure rolls back the entire operation.

| HTTP | Meaning |
| --- | --- |
| 400 | Invalid/nonpositive Advance ID, or invalid stored amount. |
| 401 | Missing/invalid authentication. |
| 403 | Role is not ACCOUNTING. |
| 404 | Advance not found. |
| 409 | Not APPROVED, Project CLOSED, already paid, or another ACTIVE Advance in the Project. |
| 500 | Unexpected persistence failure; database details are not exposed. |

## Deployment and tests

Apply the new paired `0006_create_advance_disbursements` migration using the
normal migration process before enabling this endpoint, then restart/deploy the
server. This uses version 0006 already reserved by the central migration plan;
existing migrations are unchanged. No application migration was run during
implementation. Rolling back 0006 removes payment records, so disable this
capability before rollback.

`tests/advance_disbursement*_test.go` cover service/HTTP permissions, trusted
actor, exact money, statuses, history, rollback, bound repository writes and
known database conflict mappings. PostgreSQL tests require an explicit
`ERBM_MIGRATION_TEST_DATABASE_URL` for a disposable database; they use isolated
schemas and test simultaneous confirmations, transaction rollback and migration
up/down. They never load application configuration.

## Verification on 17 November 2026

Passed:

```sh
gofmt -w internal/model/advance.go internal/model/advance_disbursement.go internal/repository/advance_disbursement.go internal/service/advance.go internal/service/advance_disbursement.go internal/handler/advance.go internal/handler/advance_disbursement.go internal/router/router.go dto/error.go
gofmt -w tests/advance_disbursement_test.go tests/advance_disbursement_postgres_test.go tests/advance_disbursement_repository_test.go
go test ./tests -run '^TestAdvanceDisbursement' -count=1 -v
go test ./tests -run '^TestAdvance(Disbursement|PMReview|History)' -count=1 -v
go vet ./...
go build ./cmd/server ./cmd/migrate
swag init -g cmd/server/main.go -o docs
git diff --check
```

The first targeted run skipped PostgreSQL cases without a test database URL.
The second targeted run supplied `ERBM_MIGRATION_TEST_DATABASE_URL` pointing to
a newly created disposable local PostgreSQL database. All selected tests,
including PostgreSQL concurrency, rollback and migration up/down, passed. The
temporary database and temporary login role were removed afterward; application
data/migration state was not changed. Swagger and Postman JSON contract checks
also passed. Swaggo emitted its existing nonfatal warning about no Go files in
the repository root.

`go test ./...` failed before and after implementation on existing
`TestActivationDeliveryFailureIsLoggedWithoutInvitation` and
`TestActivationDeliverySuccessIsLoggedWithoutInvitation`, which expect email
logs that the current activation service does not write. The sandboxed final
run also could not bind an OCR test port; rerunning outside the sandbox removed
that failure and left only the two activation logging failures. These unrelated
tests/behavior were preserved.

The repository's existing `.gitignore` ignores `/tests/`, including these new
test files. They are present locally; include them explicitly if committing.
