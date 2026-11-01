# Submit an Advance request

`POST /api/v1/advances/{id}/submit` requires a Bearer access token and a
positive int64 Advance ID. No request body is required.

The authenticated PC must be the creator and the current assigned PC of the
Project. The Project must not be CLOSED. Only DRAFT can transition to
PENDING_PM_APPROVAL. A returned request must first be edited through
`PATCH /api/v1/advances/{id}` to become DRAFT. Repeated submission and every
other status return 409.

Submission uses the stored purpose and exact positive THB amount. Client body
or query fields cannot change the amount, status, code, Project or actor.
The successful 200 response uses the existing Advance detail DTO and the
standard `success`, `message: "OK"`, `data` envelope, with the pending status
and latest `submitted_at`.

The service locks Project then Advance, rechecks ownership, current assignment,
Project status and request status, then writes submission/update metadata and
status. The SUBMIT event records actor, previous/resulting status, timestamp
and immutable before/after snapshots in the same transaction. Persistence,
audit, response-read or commit failures roll everything back. Concurrent
duplicate submissions produce one successful transition and one submitted round.

## Read submitted rounds through history

`GET /api/v1/advances/{id}/history` now combines action history and submitted
request contents. Each SUBMIT has `revision`, `amount`, and `purpose` from that
round, together with its actor, event time and status transition. Full audit
snapshots are not exposed. The separate revisions endpoint has been removed.

The first successful submission is revision 1, counted per Advance in event-ID
order. Each successful resubmission adds one revision. CREATE, UPDATE, RETURN,
APPROVE and failed submissions do not allocate revision numbers. Saving or
approving cannot change the contents of earlier submitted rounds.

For example, after editing 25,000 THB to 18,500 THB and resubmitting, history
contains SUBMIT revision 1 with amount `"25000.00"` and revision 2 with
`"18500.00"`. Each purpose is also the wording submitted in that round.
An Advance never submitted has no SUBMIT events but may still have CREATE and
UPDATE events. See [History](advance-history.md) for the response, visibility,
frontend mapping and verification commands.

## Errors

400 invalid ID or incomplete/invalid stored request, 401 unauthenticated,
403 role/creator/assignment denied, 404 Advance missing, 409 non-DRAFT or closed
Project, 500 unexpected persistence error without database details.

## Storage

Uses existing submission columns and `advance_workflow_events` in migration
0005. Stored snapshots are preserved. No schema changes or migrations are needed.
