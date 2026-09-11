---
description: Implement or review ERBM authentication, authorization,
  OTP, roles, and Empeo employee synchronization
name: authentication
---

# Authentication

Use this skill for login, JWT, OTP, account activation, password reset,
authorization middleware, roles, employees, and Empeo synchronization.

## Domain

Relevant areas may include:

``` text
Empeo
  -> employees
  -> users
  -> OTP activation / password setup
  -> login
  -> authenticated API access
```

Follow the actual repository schema and confirmed requirements.

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

External employee status and local application authorization are
separate concerns. Do not overwrite local role/business data merely
because an external employee payload omits it.

## Empeo synchronization

-   Use the external employee identifier as the stable integration
    identifier.
-   Make synchronization idempotent where practical.
-   Do not infer undocumented employment-status meanings.
-   Handle partial failures without silently leaving inconsistent state.
-   Never expose or log integration credentials.

## Tests

For real authentication changes, cover relevant cases such as:

-   valid credentials
-   invalid credentials
-   missing credentials
-   expired/invalid token
-   inactive/deactivated account
-   invalid role/authorization
-   expired/invalid OTP
-   lockout/attempt behavior when implemented
