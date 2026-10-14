---
name: migration-plan
description: Use when planning ERBM PostgreSQL schema changes across migration versions, deciding migration boundaries, or dividing migration work among developers before writing SQL.
---

# Migration Plan

Turn confirmed requirements and the existing schema into migration units
that can be assigned, applied, verified, and rolled back in dependency
order. Choose boundaries by capability and integrity, not table count or
the number of developers.

## Scope

Generate a plan in the user's language. Planning alone does not authorize
SQL implementation, database access, migrations, containers, or changes
to API contracts. Use `database-migration` when implementing SQL and
`advance-workflow` when planning Advance, Expense, OCR, or Closing.

## Shared ERBM table blueprint

For a complete database-table plan or work allocation that must use the
same schema, read the repository's shared
[ERBM database plan](../../../docs/migration-plans/erbm-database-plan.md).
Use its table/column names, data dictionary, version boundaries, and
decision register as the starting proposal. Keep unresolved decisions
visible; the blueprint does not approve business rules or authorize SQL.
Update the shared plan when interfaces change instead of producing
conflicting schema copies in separate assignments. Recheck the current
repository before treating its baseline as current.

## Establish the baseline

1. Run `git status --short`; preserve existing work. Read `AGENTS.md`,
   relevant requirements, migration pairs, `cmd/migrate/main.go`, creation
   tooling, and affected model/repository/service/DTO code.
2. Inventory existing objects and dependencies; plan only the required
   delta. Separate repository history from deployed state. Applied
   versions and real data remain unknown without authorized evidence.
3. Inspect filename sorting, version identity, transaction scope,
   discovery of templates, rollback selection, and target-version support.
   Prefer the active naming convention; report conflicts with skill text
   or tooling rather than silently changing either. Never renumber
   historical migrations.
4. Classify requirements as confirmed, proposed, or unresolved, citing
   their sources. Ask only questions that block a particular unit;
   continue independent planning. Unresolved fields, statuses, or rules
   make that unit provisional, not ready to implement.

## Choose version boundaries

Each version needs one purpose, known dependencies, acceptance evidence,
and a recovery strategy. Keep related tables, foreign keys, and required
constraints/indexes together when they form one usable capability.

Every committed prefix must preserve the invariants required by the
application deployed at that point. For a new Advance capability,
include its required concurrency protection in the same version. A later
guard is acceptable only with an explicit deployment gate that prevents
use until protection exists; assess existing data before adding it.

Split when dependency cycles, data volume, locking, incompatible app
versions, or separate release gates justify it. For changes to populated
tables, consider expand, backfill/validate, then contract; state the
observable condition that permits each stage. Avoid both a whole-system
migration and one migration per table as automatic defaults.

For each unit assess exact monetary types, existing rows, nullability,
defaults, FK deletion behavior, locks, and app compatibility. Cross-row
financial/workflow checks belong in transactional services/repositories;
do not invent a row `CHECK` for the Closing equation. Do not encode
unconfirmed rules. Flag SQL incompatible with the runner's transaction
model, such as concurrent index creation, as a prerequisite.

## Divide the work

Map each version to one owner and reviewer role, its file pair, schema
interface, prerequisites, and definition of done. Use role labels if
people are unspecified. Design/review tasks may run in parallel;
implementation depends on agreed interfaces. Separate that schedule from
the SQL apply order. Shared-object changes and dependent branches need
explicit integration gates.

Have one integration owner reserve unique version identifiers and
reconcile branch collisions before merge. Name unresolved units
provisionally; do not create placeholder SQL. Do not split an integrity
unit just to occupy more developers, or launch agents merely to write
the plan.

## Deliver and check

Use [references/plan-format.md](references/plan-format.md) for the output
contract and grouping example. Save requested plan artifacts under
`docs/migration-plans/` unless the user chooses another location; otherwise
present the plan in chat. Include rationale for each split or merge,
dependency/apply order, parallel work, blockers, recovery, and checks.

Review dependency order in both directions, filename collisions,
intermediate invariants, runner compatibility, populated-data handling,
and destructive rollback limits. Never describe dropped data as
recoverable merely because a down file exists. Distinguish planned checks
from executed checks; do not run a database to validate a planning task.
