---
name: authentication
description: Use when implementing or reviewing ERBM authentication, JWT sessions, activation links, role authorization, or employee/account provisioning.
---

# Authentication

Use the current implementation as the baseline for auth and employee-account
work. Read the affected vertical slice before changing behavior; confirm public
API changes and unresolved business rules as required by `AGENTS.md`.

## Source map

Paths below are relative to the repository root.

| Concern | Sources to read |
| --- | --- |
| Registered routes and role gates | `internal/router/router.go` |
| HTTP binding, cookies, trusted actor extraction | `internal/handler/auth.go`, `internal/handler/employee.go`, `dto/auth.go`, `dto/activation.go`, `dto/employee.go` |
| Login, refresh, profile, lockout | `internal/service/auth.go`, `internal/repository/auth.go` |
| JWT validation and password hashing | `internal/pkg/jwt/jwt.go`, `internal/pkg/hash/password.go`, `internal/middleware/auth.go` |
| Employee/account provisioning and updates | `internal/service/employee.go`, `internal/repository/employee.go`, `internal/repository/user.go`, `dto/user.go` |
| Activation and email delivery | `internal/service/activation.go`, `internal/repository/account_activation_token.go`, `internal/pkg/activation/token.go`, `internal/mail/` |
| Account state, schema, defaults, errors | `internal/model/`, `migrations/0001_create_auth_tables.up.sql`, `migrations/0002_add_employee_user_audit_fields.up.sql`, `migrations/0004_create_account_activation_tokens.up.sql`, `config/config.go`, `dto/error.go` |
| Project-specific authorization | `internal/service/project.go`, `internal/repository/project.go` |

## Current API

All paths have prefix `/api/v1`. JSON responses use the shared DTO envelope;
HTTP `204` has no body. Keep Swagger annotations aligned with registered routes.

| Method | Path | Credentials / behavior |
| --- | --- | --- |
| POST | `/auth/login` | Email/password; returns access token and sets refresh cookie |
| POST | `/auth/refresh` | Refresh cookie; returns a new access token |
| POST | `/auth/logout` | Clears refresh cookie; returns `204` |
| POST | `/auth/activation/set-password` | Activation token and initial password |
| POST | `/auth/activation/resend` | Public email-based resend; returns `204` for absent or non-invited accounts too |
| GET | `/me` | Bearer access token; loads current account, role, and employee profile |
| GET | `/roles` | Bearer access token with `ADMIN` role; lists assignable roles |

Employee routes all require `ADMIN`: `GET /employees`, `GET /employees/:id`,
`POST /employees`, `PATCH /employees/:id`,
`PATCH /employees/:id/deactivate`, and
`POST /employees/:id/resend-activation`.

## Account and employee lifecycle

- `employees` holds employment/profile data. `users` holds credentials, role,
  account state, login counters, and audit fields. A user is linked to one
  employee by a unique `employee_id`.
- Employment activity is derived by `Employee.IsActive()`:
  `employment_end_date == nil` means active; any date means inactive, including
  a future date. `is_active` is derived response data, not a writable field.
- Active employees created through the API receive a user with status
  `INVITED`, `password_hash = nil`, and `first_login = true`.
- Successful initial-password activation changes `INVITED` to `ACTIVE` and
  stores a bcrypt password hash. Login requires `ACTIVE`.
- Deactivation records an end date and sets any linked user to `DEACTIVATED`
  in the same transaction. The date must not precede `date_hired`; a repeated
  deactivation returns a conflict.
- `first_login` is currently not cleared by activation or login. Do not infer
  an additional OTP or password-change step from this flag.
- Seeded role codes are `EMPLOYEE`, `ADMIN`, `PC`, `PM`, and `ACCOUNTING`.
  Resolve role IDs from the database; HR `position` does not grant permissions.

## Employee provisioning and updates

`POST /employees` accepts a non-empty JSON **array**, even for one employee.
Each item requires `first_name`, `last_name`, `position`, `email`, `date_hired`,
and `role_code`; `employment_end_date` is optional. Dates use `YYYY-MM-DD`,
and an end date must not precede the hire date.

```json
[
  {
    "first_name": "Ada",
    "last_name": "Lovelace",
    "position": "Project Coordinator",
    "email": "ada@example.com",
    "date_hired": "2026-01-01",
    "role_code": "PC"
  }
]
```

- The DTO mapper trims names/position and lowercases/trims email. Role codes
  are trimmed, uppercased, and checked against the `roles` table.
- Require an explicit valid role for every item, including inactive employees.
  Inactive employees receive a profile only, without a login account or invite.
- The handler detects duplicate normalized emails within the array. Database
  unique constraints protect both employee and user emails under concurrency;
  conflicts map to `ErrEmployeeEmailExists` (`409`).
- The service creates employees, eligible users, and activation-token hashes
  in one transaction. A database failure rolls back the entire batch.
- Send invitation emails after commit. Initial creation suppresses SMTP
  failure and keeps committed accounts/tokens available for later resend.
  Delivery stops on the first failure, so later batch recipients are not
  attempted in that call.
- `created_by` and `updated_by` come from the authenticated administrator.
- Employee PATCH supports pointer fields for first name, last name, position,
  and email. Omitted fields remain unchanged; an empty update is rejected.
  `date_hired`, role, and employment end date are not PATCH fields. Email
  updates synchronize the employee and any linked user in one transaction.

This is atomic JSON batch creation. There is currently no CSV/XLSX upload,
partial-success import, row-level error report, or request row-count/upload
limit. Repository batch size `100` controls insert batching, not API capacity.
For a new file-import feature, confirm format and failure semantics before
defining its contract, then share provisioning rules and add explicit limits.

## Activation links

Activation uses a random email-link token, not an OTP code. The token helper
generates 32 random bytes, encodes them as URL-safe base64, and stores only a
SHA-256 digest in `account_activation_tokens`. Its lifetime is fixed at 24
hours. Plaintext stays in process memory until post-commit email delivery;
the frontend URL receives it as the `token` query parameter.

- `SetInitialPasswordRequest` contains `token` and `password`. The DTO
  validates password length `8..72`; bcrypt also imposes a 72-byte input limit.
- Initial-password activation locks the token and user rows in a transaction,
  rejects expired, used, invalidated, or non-`INVITED` tokens/accounts, updates
  the password/status, and marks the token used atomically. Invalid activation
  returns `ErrInvalidActivationToken` (`400`). Current token/user lookup
  failures also map to this error, including database lookup failures.
- Resend locks the user, requires `INVITED`, invalidates all outstanding
  tokens, and stores a replacement before sending email after commit.
- Public resend lowercases/trims email and returns the same `204` for a
  missing, `ACTIVE`, or `DEACTIVATED` account. It suppresses SMTP failures;
  validation and internal database failures can still return errors.
- ADMIN resend by employee ID reports a missing account (`404`), an account
  not awaiting activation (`409`), or SMTP delivery failure (`502`). A delivery
  failure leaves the replacement token committed and old links invalidated.

Use `mail.Sender` for delivery and `FRONTEND_ACTIVATION_URL` for the frontend
link. SMTP settings are read by `config.Load()`; an unconfigured sender fails
delivery without preventing database provisioning.

## Login and JWT sessions

- Login binds required email/password, trims the email, and performs an exact
  database lookup. Unlike provisioning/update/public resend, login currently
  does not lowercase email. Preserve or explicitly address this distinction
  when working on email normalization.
- Password verification uses bcrypt. Unknown-email login uses a dummy hash
  comparison. Unknown email, missing password hash, wrong password, a live
  lock, and non-`ACTIVE` status return `ErrInvalidCredentials` (`401`). The
  `ErrAccountLocked` sentinel exists but is not returned by this login flow.
- Failed password attempts update counters atomically. Defaults are 10 failed
  attempts and a 3-minute lock, configured by `AUTH_MAX_FAILED_LOGIN_ATTEMPTS`
  and `AUTH_LOGIN_LOCK_DURATION`. An expired lock starts a new failed-attempt
  window; successful login resets counters/lock and updates `last_login_at`.
- Access and refresh JWTs use HS256 with distinct configured secrets of at
  least 32 bytes; never hard-code credentials. Validate signature/algorithm,
  issuer, expiry, issued-at, positive user ID, matching subject, token ID/type,
  and configured maximum lifetime; access tokens additionally require a role.
- Access-token claims include `user_id`, `role`, and `token_type = access`.
  Refresh claims contain identity and `token_type = refresh`, without a role.
  Default TTLs are 15 minutes (`JWT_ACCESS_TOKEN_TTL`) and 168 hours
  (`JWT_REFRESH_TTL`).
- Login returns `access_token` and `expires_in` in seconds. The refresh token
  is stored only in the `refresh_token` cookie: `HttpOnly`, `SameSite=Lax`,
  path `/api/v1/auth`, lifetime from refresh TTL, and `Secure` controlled by
  `REFRESH_COOKIE_SECURE`.
- Refresh reads the cookie, validates it, reloads account status/current role,
  requires `ACTIVE`, and issues an access token. It does not rotate the refresh
  token or check login lockout. Logout clears the cookie and returns `204`;
  there is no server-side token revocation/session store.

## Authorization and current limits

`RequireAuth` validates a Bearer access JWT and sets `userID` (`int64`) and
`role` (`string`) in Gin context. Extract these with guarded assertions;
derive actor/audit IDs from that context. `RequireRole` compares allowed role
codes case-insensitively and returns `401` for missing/malformed role context
or `403` for a disallowed role.

Middleware currently trusts the verified JWT claims without reloading account
status or role. Deactivation or a role change therefore does not immediately
invalidate an already-issued access token. `/me` reloads profile/status/role
from the database but does not reject a non-`ACTIVE` status. Keep these limits
explicit when changing authorization; do not claim immediate revocation.

Project routes allow `ADMIN`, `PM`, `PC`, and `ACCOUNTING` to read, and
`ADMIN`, `PM`, and `PC` to write. Membership and field-level permissions belong
in `ProjectService`; role middleware alone does not authorize a specific
project. `EMPLOYEE` has no employee-management or project-route access.

## Scope and verification

No OTP verification, public registration, password reset/change, account
reactivation, role-editing endpoint, or initial ADMIN bootstrap command is
implemented. The company has no Empeo API access; do not add integration or
sync without explicitly changed requirements. Schema changes must follow
`AGENTS.md` and the database-migration skill; never edit shared applied
migrations.

For backend behavior changes, add tests in root-level `tests/`. Existing
`tests/activation_service_test.go` covers activation, token reuse/expiry,
resend invalidation, SMTP failure after commit, rollback, token hashing, and
login after activation. Extend coverage as relevant for login lockout, JWT
validation, refresh/cookies, trusted identity/roles, normalized email conflicts,
inactive employee provisioning, PATCH synchronization, and deactivation.

Run `gofmt` on changed Go files, `go test ./...`, `go vet ./...`, and
`go build ./cmd/server ./cmd/migrate` for implementation changes. Keep database
access in repositories with `WithContext(ctx)` and multi-step account changes
transactional. Never expose passwords, activation tokens, JWTs, or secrets in
logs, responses outside their intended credential flow, or import files.
