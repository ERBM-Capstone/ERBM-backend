---
description: Create or review PostgreSQL schema migrations for the ERBM
  backend
name: database-migration
---

# Database Migration

Use this skill whenever a task changes the database schema.

## Naming

Create paired migrations:

``` text
YYYYMMDDHHMMSS_description.up.sql
YYYYMMDDHHMMSS_description.down.sql
```

The base names must match exactly.

## Rules

-   Use PostgreSQL-compatible SQL.
-   Make the down migration safely reverse the up migration.
-   Respect dependency order for foreign keys and dependent objects.
-   Use exact numeric/decimal types for monetary data; never use
    floating point.
-   Add indexes and constraints only for confirmed requirements.
-   Do not encode unresolved business rules into database constraints.
-   Consider concurrency when enforcing workflow invariants.
-   Never edit a migration already applied to a shared environment.

## Running migrations

Migration commands mutate the configured database. Run them only when
the task requires it and the target database is known to be safe.

Never inspect or print `.env` to obtain credentials.

## Review checklist

-   up/down names match
-   rollback reverses the change
-   foreign-key order is safe
-   money types are exact
-   indexes match expected access patterns
-   no secrets or environment-specific data are included
