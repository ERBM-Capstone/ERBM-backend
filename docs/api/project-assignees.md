# Project PM/PC dropdowns

GET `/api/v1/projects/assignees` provides the existing accounts for the PM and
PC dropdowns on the create Project form. It requires a Bearer access token with
the `ADMIN`, `PM`, or `PC` role. `ACCOUNTING` and `EMPLOYEE` receive 403;
unauthenticated requests receive 401.

## Query

| Parameter | Required | Meaning |
| --- | --- | --- |
| `role` | Yes | `PM` or `PC`, uppercase. |
| `q` | No | Case-insensitive search in full name or account email; at most 255 characters. Leading/trailing whitespace is trimmed. |
| `page` | No | Page number, default 1. |
| `page_size` | No | Items per page, default 20, maximum 100. |

```http
GET /api/v1/projects/assignees?role=PM&page=1&page_size=20
Authorization: Bearer <access_token>
```

```http
GET /api/v1/projects/assignees?role=PC&q=Somchai&page=1&page_size=20
Authorization: Bearer <access_token>
```

Only users with account status `ACTIVE` and the requested application role
appear. Employee position titles do not determine eligibility. `INVITED` and
`DEACTIVATED` accounts, and employees without accounts, do not appear. Existing
Project assignments do not exclude a user because a PM or PC may manage many
Projects. Results sort by first name, last name, then user ID for stable paging.
Search uses PostgreSQL `ILIKE`; `%` and `_` retain their wildcard meanings.

## Response

```json
{
  "success": true,
  "message": "Project assignees retrieved",
  "data": [
    {
      "user_id": 100,
      "first_name": "Pat",
      "last_name": "Manager",
      "email": "pat@example.com"
    }
  ],
  "meta": {
    "page": 1,
    "per_page": 20,
    "total": 1
  }
}
```

An empty result is `data: []` with `meta.total: 0`. A page beyond the last page
also returns `data: []`, while `meta.total` still counts all matching users.
Invalid/missing `role`, invalid paging, or an oversized search returns 400.
Database failures return the standard 500 response without database details.

## Connect the create form

Use `user_id` as each option's value and the full name as its label. Email can
distinguish people with the same name. Use `/api/v1/me` for the creator's
identity and role, then follow the existing creation rules:

| Creator | PM field | PC field |
| --- | --- | --- |
| ADMIN | Select one PM from `?role=PM`; required. | Select at most one PC from `?role=PC`; optional. |
| PM | Show the authenticated PM as fixed; the server assigns them automatically. | Select at most one PC from `?role=PC`; optional. |
| PC | Select one PM from `?role=PM`; required. | Show the authenticated PC as fixed; the server assigns them automatically. |

This lookup returns eligible accounts across the system; it does not change
creation permissions. On POST `/api/v1/projects`, submit the selected values
as `pm_user_id` and `pc_user_id`. A PC creator may omit `pc_user_id` or submit
their own user ID. ADMIN/PM may omit it or submit null for no assigned PC.

For example, an ADMIN selection submits:

```json
{
  "name": "Project A",
  "pm_user_id": 100,
  "pc_user_id": 25
}
```

The create/update service checks role and ACTIVE status again when saving.
An account may have changed after the form loaded; handle the existing 400
assignment validation response by refreshing the dropdown options. No schema
or migration change is required.

## Verification

`tests/project_assignee_test.go` covers service authorization, input validation,
trusted actor context, response fields, empty arrays and pagination metadata,
registered route permissions, and parameterized repository queries that retain
role/status/search filters for both the count and the page. Its opt-in PostgreSQL
test checks ACTIVE-only eligibility, role versus position, profiles without
accounts, existing assignments, full-name/email search, and stable pagination.
The database test uses only the explicitly configured disposable
`ERBM_MIGRATION_TEST_DATABASE_URL` and an isolated schema; it skips when that
configuration is absent. It never reads application configuration or `.env`.
