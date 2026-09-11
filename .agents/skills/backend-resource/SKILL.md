---
description: Implement or modify an ERBM REST API resource across the Go
  backend vertical slice
name: backend-resource
---

# Backend Resource

Use this skill when adding or modifying an API resource.

## Read first

Inspect the relevant resource and an existing reference implementation:

1.  model
2.  DTO
3.  repository
4.  service
5.  handler
6.  router
7.  errors
8.  migrations
9.  tests

## Vertical slice

Normally consider:

``` text
internal/model/<resource>.go
dto/<resource>.go
internal/repository/<resource>.go
internal/service/<resource>.go
internal/handler/<resource>.go
internal/router/router.go
```

Add errors, migrations, and tests when required.

## Handler and DTO

-   Use explicit JSON, URI, and Gin validation tags.
-   Use `ShouldBindUri` for route parameters.
-   Use `ShouldBindJSON` for JSON bodies.
-   Convert binding failures to the project's bad-request error.
-   Return DTOs rather than persistence models.
-   Keep handlers thin.
-   Derive actor identity from authenticated context.
-   Keep Swagger annotations consistent with registered routes.

## Service

Services own:

-   business validation
-   defaults
-   authorization
-   workflow transitions
-   cross-repository orchestration
-   financial invariants

## Repository

-   Use GORM with `WithContext(ctx)`.
-   Apply soft-delete rules where applicable.
-   Check affected rows when missing records matter.
-   Use transactions and locking for concurrency-sensitive writes.
-   Raw SQL is acceptable when clearer or safer.

## Tests

Cover relevant cases:

-   success
-   invalid input
-   unauthenticated
-   unauthorized
-   not found
-   invalid workflow state
-   soft-deleted record
-   business invariant violation
