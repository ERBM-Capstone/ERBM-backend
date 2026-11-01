# Advance action history

`GET /api/v1/advances/{id}/history` returns the recorded actions for one
Advance. Use it for the timeline on the detail/approval screen, including the
reason when PM returns a request and the revision number, amount and purpose
of each submitted round. Submission data and actions share this endpoint.

```http
GET /api/v1/advances/7/history
Authorization: Bearer <access-token>
```

No request body or pagination is required. All recorded events are returned,
newest first by `occurred_at DESC, id DESC`. Event IDs break timestamp ties.

```json
{
  "success": true,
  "message": "OK",
  "data": [
    {
      "id": 3,
      "action": "RETURN",
      "actor": {
        "user_id": 30,
        "first_name": "Manager",
        "last_name": "A"
      },
      "previous_status": "PENDING_PM_APPROVAL",
      "resulting_status": "RETURNED_FOR_REVISION",
      "reason": "กรุณาชี้แจงรายละเอียดค่าเดินทาง",
      "revision": null,
      "amount": null,
      "purpose": null,
      "occurred_at": "2026-10-31T03:00:00Z"
    },
    {
      "id": 2,
      "action": "SUBMIT",
      "actor": {
        "user_id": 25,
        "first_name": "Coordinator",
        "last_name": "B"
      },
      "previous_status": "DRAFT",
      "resulting_status": "PENDING_PM_APPROVAL",
      "reason": null,
      "revision": 1,
      "amount": "25000.00",
      "purpose": "ค่าใช้จ่ายโครงการ",
      "occurred_at": "2026-10-31T02:00:00Z"
    },
    {
      "id": 1,
      "action": "CREATE",
      "actor": {
        "user_id": 25,
        "first_name": "Coordinator",
        "last_name": "B"
      },
      "previous_status": null,
      "resulting_status": "DRAFT",
      "reason": null,
      "revision": null,
      "amount": null,
      "purpose": null,
      "occurred_at": "2026-10-31T01:00:00Z"
    }
  ]
}
```

`action` includes all stored workflow events: currently `CREATE`, `UPDATE`,
`SUBMIT`, `RETURN`, and `APPROVE`. Saving a draft is an `UPDATE`; resubmitting
is another `SUBMIT`. Failed operations and reads create no history entries.
`id` is the audit event ID, not a revision number or the Advance ID.
Timestamps use RFC3339 with fractional seconds when present. Actor IDs are
recorded at the action; names come from the actor's current employee profile,
including actors whose accounts have since been deactivated.

`previous_status` is null for creation. `reason` is null for actions with no
reason. For `SUBMIT`, `amount` is the exact THB decimal string from that
event's immutable `after_data.amount`, rather than the current Advance amount.
For example, after editing 25,000 THB to 18,500 THB and submitting again, the
older `SUBMIT` still has `"amount": "25000.00"` and the new one has
`"amount": "18500.00"`. Frontend can render “ยื่นคำขอ (ยอด ฿25,000.00)”
directly from the event without matching it to a separate revision.

`purpose` is also read from that same immutable snapshot. Historical wording
does not change when the current request is edited. `revision` starts at 1 per
Advance and counts successful `SUBMIT` events by event ID ascending. CREATE,
UPDATE, RETURN, APPROVE and failed/duplicate submissions allocate no revision.
Revision numbering is independent of timeline display order and timestamps.

`revision`, `amount` and `purpose` are null for all other actions. Missing
snapshot amount/purpose fields remain null. No fallback to current request
data is made. Full before/after snapshots, credentials and internal Project
membership fields are not included. The frontend can translate actions and
statuses for display using these codes.

An existing Advance with no events returns `data: []`. Missing Advances
return 404; missing history alone does not. Reading does not modify status.

## Access

Uses the existing GET detail policy:

- A Bearer access token is required.
- ADMIN and ACCOUNTING may read all Projects, subject to private drafts.
- PM must currently manage the Project; PC must currently be assigned to it.
- A current DRAFT and its history are private to its creator for every role.
  This also applies after a returned request is edited back to DRAFT.
- Closed Advances and Projects may still be read under the same policy.

Errors use the standard response envelope: 400 invalid/nonpositive/overflowing
ID, 401 unauthenticated, 403 role/assignment/private-draft denial, 404 Advance
missing, 500 unexpected read failure without internal database details.

## Frontend usage and consolidated API

Load `GET /api/v1/advances/{id}` for the current form and this history endpoint
for the timeline, return reason and prior submission contents. Filter history
by `action: "SUBMIT"` to display submitted rounds; each item already contains
`revision`, `amount` and `purpose`. Actor and event time identify who submitted
that round and when. To compare old and new wording/amount, compare these
fields from the relevant submissions.

The separate `GET /api/v1/advances/{id}/revisions` route has been removed and
returns 404. Its DTO, handler, service and repository read path were removed.
Existing stored snapshots remain intact and are read through `/history`.
See [Submit](advance-submit-revisions.md) for saving/submitting the request.

## Storage and verification

Reads existing `advance_workflow_events`, user/employee profiles and current
Advance/Project visibility in one SQL statement with the request context,
bound SUBMIT actions and bound Advance ID. Only submitted amount and purpose
are extracted from snapshots; full snapshots are not loaded. The service
assigns per-Advance revision numbers while retaining the timeline order. No
schema changes are required.

Local tests in the ignored `tests/` folder cover service authorization, actor
guards, route authentication/validation, error responses, nullable fields,
empty history, exact amount text, historical purpose, stable revision numbers,
public DTO fields and the Swagger contract. They also verify the removed
revisions route returns 404 and is absent from Swagger.
Repository tests check the single contextual read, bound parameters,
snapshot-based SUBMIT amount, all-action scope and stable order.
Optional PostgreSQL history tests in `advance_pm_review_postgres_test.go`
exercise the complete return/edit/resubmit/approve timeline, actor names,
timestamp ties, amount/purpose retained across edits/resubmissions, nullable
submission fields for other actions, scope, and empty versus missing Advances.
They use an isolated
schema only when `ERBM_MIGRATION_TEST_DATABASE_URL` is explicitly configured.

### Checks run on 2026-10-31

Passed:

```sh
go test tests/advance*_test.go tests/service_actor_guard_test.go tests/swagger_test.go -count=1
go vet ./cmd/... ./config/... ./dto/... ./internal/... ./docs/...
go build ./cmd/server ./cmd/migrate
go run github.com/swaggo/swag/cmd/swag init -g cmd/server/main.go -o docs
git diff --check
git diff --cached --check
```

Changed Go files were formatted with `gofmt -w` and checked with `gofmt -l`.
Swagger generation emitted the existing nonfatal warning about no Go files
in the repository root. JSON checks verified the documented response, Postman
request, Swagger Bearer security, response codes and public response schema,
including nullable `revision`, `amount` and `purpose`, and absence of the
removed revisions route/response.
`git check-ignore -v tests/advance_history_test.go tests/advance_history_repository_test.go`
confirmed the new tests stay ignored as requested.

Attempted full checks:

```sh
go test ./...
go vet ./...
```

Both remain blocked by the existing local `project_code_test.go:78,80`, which
references `CreatedAt` on the Project response. The Project tests and contract
were left unchanged. Submission tests were migrated to the consolidated
history response as part of this change, removing their earlier compile errors.

```sh
go test tests/advance*_test.go -run '^TestAdvance(HistoryPostgres|SubmitPostgres)' -count=1 -v
```

The PostgreSQL history/submission tests skipped because the explicit
disposable database URL was not configured. Live database mapping and timeline
behavior therefore remain unverified in this session. No migrations were run.
