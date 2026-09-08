# Starter Code Generation Prompt

Use the template below whenever you need to scaffold a **new feature/resource** in this repository.
Replace every `<Placeholder>` with real values before sending.

---

## Prompt Template

```
You are generating Go starter code for the **project-hub-backend** service.
Strictly follow the conventions described below — do not invent new patterns.

---

### 1. Context

- Module : github.com/your-org/project-hub-backend
- Language: Go 1.21+
- Framework: Gin (github.com/gin-gonic/gin)
- Database : PostgreSQL via pgx/v5 + pgxpool (no ORM)
- Auth     : Supabase JWT; claims extracted by middleware (middleware.ContextKeyUserID)
- Docs     : Swagger via swaggo/swag (@Summary, @Router, etc.)

---

### 2. Resource to scaffold

| Field | Value |
|---|---|
| Resource name (singular, PascalCase) | <ResourceName>          |
| Resource name (plural, snake_case)   | <resource_names>        |
| DB table name                        | <table_name>            |
| Route prefix                         | /api/v1/<route_prefix>  |
| Required roles for write operations  | admin, editor           |
| Parent resource (if nested, else —)  | <ParentResource or "—"> |

---

### 3. Model fields

Define the struct fields for `internal/model/<resource_name>.go`:

| Field name (Go) | Type        | DB column       | JSON key       | Notes            |
|---|---|---|---|---|
| ID              | string      | id (UUID)       | id             | PK, set by DB    |
| <FieldName>     | <GoType>    | <column>        | <json_key>     | required / omit? |
| ...             | ...         | ...             | ...            |                  |
| CreatedAt       | time.Time   | created_at      | created_at     | set by DB        |
| CreatedBy       | *string     | created_by      | created_by     | omitempty        |
| UpdatedAt       | time.Time   | updated_at      | updated_at     | set by DB        |
| UpdatedBy       | *string     | updated_by      | updated_by     | omitempty        |
| DeletedAt       | *time.Time  | deleted_at      | deleted_at     | soft-delete      |
| DeletedBy       | *string     | deleted_by      | deleted_by     | omitempty        |

---

### 4. Files to generate

Generate **all** of the following files, in order:

#### 4-A. `internal/model/<resource_name>.go`
- Package `model`
- One main struct named `<ResourceName>` with JSON tags.
- Optional status constants as `type <ResourceName>Status string` if the resource has a status field.
- Follow exactly:

```go
package model

import "time"

type <ResourceName> struct {
    ID        string     `json:"id"`
    // ... your fields ...
    CreatedAt time.Time  `json:"created_at"`
    CreatedBy *string    `json:"created_by,omitempty"`
    UpdatedAt time.Time  `json:"updated_at"`
    UpdatedBy *string    `json:"updated_by,omitempty"`
    DeletedAt *time.Time `json:"deleted_at,omitempty"`
    DeletedBy *string    `json:"deleted_by,omitempty"`
}
```

#### 4-B. `dto/<resource_name>.go`
- Package `dto`
- `<ResourceName>ID` struct with `uri:"id" binding:"required,uuid"` for path params.
- `Create<ResourceName>Request` with `json` + `binding` tags.
- `Update<ResourceName>Request` with `json` + `binding` tags.
- `<ResourceName>Response` struct (public-facing shape, may differ from model).
- Mapper functions:
  - `ToCreate<ResourceName>(params *Create<ResourceName>Request) *model.<ResourceName>`
  - `ToUpdate<ResourceName>(params *Update<ResourceName>Request) *model.<ResourceName>`
  - `To<ResourceName>Response(item *model.<ResourceName>) <ResourceName>Response`
- `DeleteResponse` is already global — do not redefine it.
- Follow existing patterns in `dto/project.go`.

#### 4-C. `internal/repository/<resource_name>.go`
- Package `repository`
- Struct `<ResourceName>Repository` with `pool *pgxpool.Pool`.
- Constructor `New<ResourceName>Repository(pool *pgxpool.Pool) *<ResourceName>Repository`.
- Methods (raw SQL, pgx, no ORM):
  - `List(ctx)` — `WHERE deleted_at IS NULL ORDER BY updated_at DESC`
  - `GetByID(ctx, id)` — return `dto.ErrNotFound` on `pgx.ErrNoRows`
  - `Create(ctx, item, createdBy)` — `INSERT … RETURNING`
  - `Update(ctx, item, updatedBy)` — check soft-delete first; wrap in `tx.Begin / tx.Commit / defer tx.Rollback`
  - `Delete(ctx, id, deletedBy)` — soft-delete; return `dto.ErrAlreadyDeleted` if `RowsAffected() == 0`
- Always guard type assertions with `, ok`.

#### 4-D. `internal/service/<resource_name>.go`
- Package `service`
- Struct `<ResourceName>Service` wrapping the repository.
- Constructor `New<ResourceName>Service(repo *repository.<ResourceName>Repository) *<ResourceName>Service`.
- Thin delegation methods; business logic (default status, etc.) goes here — not in handler or repo.
- Follow the pattern in `internal/service/projects.go`.

#### 4-E. `internal/handler/<resource_name>.go`
- Package `handler`
- Struct `<ResourceName>Handler` with `service *service.<ResourceName>Service`.
- Constructor `New<ResourceName>Handler(svc *service.<ResourceName>Service) *<ResourceName>Handler`.
- Methods: `List`, `GetByID`, `Create`, `Update`, `Delete`.
- Every method must have **full Swagger comments** (`@Summary`, `@Description`, `@Tags`, `@Produce`, `@Accept`, `@Security BearerAuth`, `@Param`, `@Success`, `@Failure`, `@Router`).
- Use `c.ShouldBindJSON` / `c.ShouldBindUri` for input; `dto.HandleError(c, err)` for errors.
- Extract `userID` from context with the `, ok` guard:

```go
userID, ok := c.Get(middleware.ContextKeyUserID)
if !ok || userID == nil {
    dto.HandleError(c, dto.ErrUnauthorized)
    return
}
```

- Follow the pattern in `internal/handler/projects.go`.

#### 4-F. Router registration snippet (add to `internal/router/router.go`)

```go
// <ResourceName>
<resourceNames>Repo    := repository.New<ResourceName>Repository(pool)
<resourceNames>Svc     := service.New<ResourceName>Service(<resourceNames>Repo)
<resourceNames>Handler := handler.New<ResourceName>Handler(<resourceNames>Svc)

<resourceNames> := v1.Group("/<route_prefix>")
{
    <resourceNames>.GET("",     <resourceNames>Handler.List)
    <resourceNames>.GET("/:id", <resourceNames>Handler.GetByID)

    <resourceNames>Write := <resourceNames>.Group("")
    <resourceNames>Write.Use(middleware.RequireRole("admin", "editor"))
    {
        <resourceNames>Write.POST("",     <resourceNames>Handler.Create)
        <resourceNames>Write.PUT("/:id",  <resourceNames>Handler.Update)
        <resourceNames>Write.DELETE("/:id", <resourceNames>Handler.Delete)
    }
}
```

#### 4-G. Error sentinels (add to `dto/error.go` only if new ones are needed)

```go
Err<ResourceName>NotFound = errors.New("<resource_name> not found")
```

Register the new sentinel in both `projectErrorMap` and `ErrorStatusMap`.

---

### 5. Conventions checklist

- [ ] Soft-delete guard (`deleted_at IS NULL`) in every LIST and GET query.
- [ ] Multi-step writes wrapped in a transaction (`pool.Begin / tx.Commit / defer tx.Rollback`).
- [ ] No bare type assertions — always use the `, ok` form.
- [ ] No hardcoded secrets — use `config.Config`.
- [ ] Return empty slices `[]T{}` not `nil` from List functions.
- [ ] Every exported function has a Go doc comment.
- [ ] Swagger `@Router` paths match the routes registered in `router.go` exactly.
- [ ] Run `go test ./... -v -cover` and fix any failures before reporting done.
- [ ] Regenerate Swagger docs with `make agent-docs swagger` after handler changes.

---

### 6. Guardrails — do NOT do

- Do not hand-edit `/docs` — regenerate via `make agent-docs swagger`.
- Do not touch `/README.md` unless the task explicitly asks for it.
- Do not read, write, or print `.env`, `/secrets`, or any API key.
- Do not write outside `/internal` and `/cmd` without asking first.
- Do not use an ORM — all queries are raw SQL via pgx.
- Do not add new `go.mod` dependencies without team confirmation.
```

---

## How to use

1. Copy the prompt template block above.
2. Fill in the **Resource to scaffold** table (section 2).
3. Fill in the **Model fields** table (section 3).
4. Paste into your AI coding assistant (Antigravity, Claude Code, etc.).
5. After code is generated, run `go test ./... -v -cover` and fix failures.
6. Run `make agent-docs swagger` to regenerate Swagger docs.
