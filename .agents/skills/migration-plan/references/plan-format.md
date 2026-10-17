# Migration plan output contract

Scale the detail to the requested change. Produce a plan someone can
turn into assignments without rediscovering dependencies or deciding
business rules. Do not manufacture a fixed number of versions.

## 1. Baseline and decisions

Record:

- Requirement sources and the requested outcome.
- Existing migration versions and relevant tables, keys, constraints,
  indexes, and model/schema mismatches.
- Repository state versus known deployed state. If deployment evidence
  was not supplied, explicitly say it was not checked.
- Current naming/generation convention, runner discovery, transaction
  behavior, and limitations relevant to this plan.
- Confirmed decisions, recommendations, and unresolved questions. For
  each question identify the affected version and useful work that can
  proceed before its answer.

Inspect the repository again for every plan. Historical ERBM examples
below are grouping examples, not a frozen schema inventory.

## 2. Version overview

Use a compact table with these columns:

| Version / proposed file stem | Purpose and objects | Depends on | Owner / reviewer | Readiness and blocker |
| --- | --- | --- | --- | --- |

Use `ready`, `provisional`, or `blocked` with a concrete reason. A ready
unit has sufficient confirmed schema decisions to write and review SQL.
A provisional unit has a candidate boundary but unresolved details. A
blocked unit cannot proceed until a named decision or prerequisite is
resolved. Specification and test-design work can be ready even when its
SQL unit is provisional.

Confirm that numeric identifiers are unique and filenames sort in
dependency order, considering other pending work. If reservation cannot
be established, label names proposed rather than claiming they are
reserved. List both intended `.up.sql` and `.down.sql` paths per version.

## 3. Version work cards

For every version specify:

- **Goal and boundary rationale:** the capability delivered, why these
  objects belong together, and why it is separate from adjacent versions.
- **Evidence and readiness:** source of each schema-affecting rule;
  decisions still required before SQL is implementable.
- **Up scope:** objects created/altered, required keys/constraints/indexes,
  relationships, and any data transformation. Proposed names, types,
  defaults, and deletion policies stay visibly proposed until confirmed.
- **Prerequisites and interface:** exact upstream objects/columns and
  agreements needed by downstream work; application release gates when
  relevant.
- **Data and deployment:** behavior on an empty and populated database,
  conflict prechecks, backfill method where needed, lock/transaction
  implications, and old/new application compatibility.
- **Down and recovery:** dependent objects removed first, schema changes
  reversed, data that cannot be restored, and any forward-fix alternative
  or backup/restore prerequisite. Distinguish schema reversal from data
  recovery. State the effect on old/new application versions.
- **Assignment:** one owner role, reviewer role, deliverables, and
  integration handoff.
- **Acceptance:** observable successful results and relevant negative
  cases; classify verification as offline review or database execution.

Give the exact apply order and reverse dependency order. If the runner
commits each file separately, describe how to resume after a later file
fails. Do not imply batch atomicity or selectable target versions when
the runner lacks them. If only a prefix is approved for release, keep
later unready migrations out of the deployable set.

## 4. Work schedule and integration

Use a task table when multiple developers are involved:

| Task / deliverable | Owner | Can start after | Parallel with | Handoff / acceptance |
| --- | --- | --- | --- | --- |

Include version reservation, interface agreement, implementation,
review, and integration verification as needed. A dependency blocks
applying SQL, but may not block writing its specification or preparing
tests. Parallel implementation of dependent units needs a stable
interface and an agreed way to obtain upstream changes. Shared files
and overlapping schema objects need one integration owner.

Do not invent developer identities, effort estimates, or deadlines.
Use supplied team size; otherwise describe roles and natural parallel
opportunities without requiring a particular team structure.

## 5. Verification and open decisions

For SQL implementation, plan appropriate verification on an authorized
disposable database: clean up chain, upgrade from the expected baseline
with representative existing rows, applicable down/up round trips,
constraint negative cases, and concurrency cases for shared invariants.
Destructive round trips need disposable fixtures. New backend tests
belong in root-level `tests/` per `AGENTS.md`.

When Go changes accompany the migrations, include:

```sh
gofmt -w <changed-go-files>
go test ./...
go vet ./...
go build ./cmd/server ./cmd/migrate
```

These are future checks unless executed. End the plan with unresolved
decisions mapped to versions and the next work that can safely start.

## Grouping example from existing ERBM migrations

`0003_create_projects` groups `projects`, its required `pm_user_id` and
nullable `pc_user_id` foreign keys to `users`, constraints, and indexes.
Each Project has one PM and at most one PC; each PM/PC can serve many
Projects. These objects support one project-management capability, so
assigning each column or index its own version would add
coordination without a meaningful release boundary. An owner can write
the pair while another developer reviews membership semantics and
prepares acceptance cases.

`0002_add_employee_user_audit_fields` is a separate stage because audit
references require `users` to exist, while `users` itself references
`employees`. Creating the base objects first resolves that dependency
cycle. This is a dependency-driven split, not a rule to separate audit
columns in every future feature.

For a proposed Advance capability, its required one-`ACTIVE` concurrency
guard must exist when the capability becomes usable. Group creation and
the guard together unless a concrete deployment gate makes a separate
stage safe. Expense/OCR/Closing boundaries still depend on their actual
confirmed relationships; these examples do not approve any new schema.
