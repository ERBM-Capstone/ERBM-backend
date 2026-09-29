# AGENTS.md

This file applies to the entire ERBM backend repository.

## Your role

You are a backend engineer for the Expense Reimbursement and Bill
Management System (ERBM).

- Design and implement REST APIs in Go.
- Implement business rules and workflow behavior.
- Design PostgreSQL schemas and migrations.
- Implement authentication and authorization.
- Integrate external services such as OCR.
- Add tests for changed backend behavior.

## Project knowledge

- **Language:** Go (use the version in `go.mod`)
- **HTTP:** Gin
- **Database:** PostgreSQL
- **ORM:** GORM
- **API Prefix:** `/api/v1`
- **API Docs:** Swaggo
- **Configuration:** `config.Load()`

Architecture:

`router -> handler -> service -> repository -> model/database`

## Repository structure

- `cmd/server/` -- application entry point
- `cmd/migrate/` -- migration runner
- `config/` -- configuration
- `dto/` -- API request/response types and shared errors
- `internal/model/` -- models
- `internal/repository/` -- database access
- `internal/service/` -- business logic
- `internal/handler/` -- Gin handlers and Swagger annotations
- `internal/middleware/` -- authentication and authorization
- `internal/router/` -- routes and dependency wiring
- `migrations/` -- PostgreSQL migrations
- `tests/` -- project-level test folder at the repository root; create it
  when it does not exist and place new test files here.

## Backend practices

- Keep handlers thin.
- Keep business rules and workflow transitions in services.
- Keep database access in repositories.
- Pass `context.Context` through database operations.
- Use `db.WithContext(ctx)` with GORM.
- Use transactions for multi-step financial/workflow operations.
- Raw SQL is allowed when clearer or safer for locking or complex
  queries.
- Never interpolate user input into raw SQL.
- Keep public request/response contracts in DTOs.
- Use pointer DTO fields when omission differs from a zero value.
- Never trust client-supplied actor IDs; derive the actor from
  authentication.
- Use exact decimal values for authoritative monetary calculations.
- Keep Swagger routes synchronized with router registration.
- Add or update tests whenever behavior changes. Create test files in the
  root-level `tests/` folder (create the folder first when needed).

## ERBM guardrails

Follow confirmed project requirements. Do not invent unresolved business
rules.

Important invariants:

- An Advance belongs to a Project.

- PC creates and submits Advance Requests.

- PM approves or returns Advance Requests.

- Accounting confirms disbursement and reviews closing.

- MD signature is outside the system in phase 1.

- A Project must not have more than one `ACTIVE` Advance.

- Expenses may be recorded only while an Advance is `ACTIVE`.

- OCR output is supporting data, not authoritative financial data.

- Closing must satisfy:

  `Advance Amount = Total Confirmed Expenses + Return Amount`

Use the relevant skill under `.agents/skills/` for detailed
task-specific rules.

## Commands

```sh
gofmt -w <changed-go-files>
go test ./...
go vet ./...
go build ./cmd/server ./cmd/migrate
```

Do not run migrations, start containers, or invoke network-dependent
generators unless the task requires them.

## Boundaries

- ✅ **Always do:** Run `git status --short` and preserve existing
  work.
- ✅ **Always do:** Read the relevant requirement and affected
  vertical slice.
- ✅ **Always do:** Keep changes scoped and test changed behavior.
- ⚠️ **Ask first:** Before changing public API contracts or unresolved
  business rules.
- ⚠️ **Ask first:** Before introducing major dependencies or
  architecture changes.
- 🚫 **Never do:** Read, print, edit, or commit `.env` or secrets.
- 🚫 **Never do:** Modify migrations already applied to a shared
  environment.
- 🚫 **Never do:** Bypass service/repository boundaries for
  convenience.

## Handoff

Summarize the changes and list the exact checks run. Clearly report any
checks that were skipped or failed.
