---
description: Implement or review ERBM authentication, authorization,
  OTP, roles, manual employee management, and bulk user imports
name: authentication
---

# Authentication

Use this skill for login, JWT, OTP, account activation, password reset,
authorization middleware, roles, manual employee management, user
provisioning, and bulk imports.

## Domain

Relevant areas may include:

``` text
Manual entry / bulk import
  -> employees
  -> users
  -> OTP activation / password setup
  -> login
  -> authenticated API access
```

Follow the actual repository schema and confirmed requirements.

The company does not have access to the Empeo API. Do not implement Empeo
API integration or synchronization unless requirements change explicitly.

## Security rules

-   Hash passwords using an appropriate password-hashing algorithm.
-   Never store plaintext passwords.
-   Store OTPs securely; do not persist plaintext OTP values.
-   Enforce OTP expiration and attempt limits when required.
-   Verify token signatures/authenticity, expiration, and required
    claims.
-   Use configured secrets/keys; never hard-code credentials.
-   Do not log passwords, OTPs, access tokens, refresh tokens, or
    secrets.
-   Derive the authenticated user from verified credentials.
-   Use guarded assertions when reading identity from Gin context.
-   Authorization must consider both role and resource/project
    relationship when required.

## Account state

Respect the confirmed user/account lifecycle in the repository. Do not
invent activation, lockout, deactivation, or password-reset behavior.

Keep employment data separate from authentication data:

-   `employees` stores the person's profile and employment state.
-   `users` stores login credentials, role, and account state.
-   Creating an employee does not automatically authorize that person.
-   Never rely on the database's default role when provisioning a user.
    Require a confirmed role assignment so an imported user cannot become
    `ADMIN` accidentally.

The confirmed employee input is:

-   first name
-   last name
-   position
-   email
-   date hired
-   optional employment end date

Email is required for this manual flow. Normalize it consistently and
enforce uniqueness safely under concurrent requests.

Derive employment activity from the confirmed rule:

``` text
employment end date is null     -> employee is active
employment end date has a value -> employee is inactive
```

Do not accept a conflicting client-supplied `is_active` value. If an
employee has a linked user, do not invent automatic account activation or
reactivation behavior; follow the confirmed account lifecycle.

The current development database may revise migration
`0001_create_auth_tables` only after its existing version has been rolled
back and only while it has not been used in a shared environment. Never edit
an applied migration in a shared environment. Employment activity is derived
from the end date and does not need a duplicate database boolean.

## Manual creation and bulk import

-   Apply the same validation and normalization rules to single-entry and
    bulk-import paths.
-   Detect duplicate emails both within an import and in the database.
-   Report import errors with row numbers and field-level reasons without
    exposing sensitive data.
-   Use transactions so employee and user creation cannot be left partially
    completed.
-   Do not put plaintext or temporary passwords in import files.
-   Confirm the import format and whether failures are atomic or partial
    before defining the public API contract.
-   Limit upload size and row count when file import is implemented.
-   Do not create login accounts for inactive employees unless that behavior
    is explicitly confirmed.

## Tests

Cover relevant cases such as:

-   valid credentials
-   invalid credentials
-   missing credentials
-   expired/invalid token
-   inactive/deactivated account
-   invalid role/authorization
-   expired/invalid OTP
-   lockout/attempt behavior when implemented
-   valid and invalid manual employee input
-   missing, malformed, and duplicate email
-   employment end date producing an inactive employee
-   duplicate rows and existing-record conflicts during import
-   transaction rollback on employee/user provisioning failure
-   import row-level errors and file-size/row-count limits
