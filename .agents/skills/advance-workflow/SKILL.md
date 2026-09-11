---
description: Implement or review ERBM Advance Request, disbursement,
  expense, OCR, and closing workflow behavior
name: advance-workflow
---

# Advance Workflow

Use the current project requirement as the source of truth.
Recommendations and open questions in analysis documents are not
automatically confirmed requirements.

## Confirmed flow

``` text
DRAFT
  -> PENDING_PM_APPROVAL

PENDING_PM_APPROVAL
  -> APPROVED
  -> RETURNED_FOR_REVISION

RETURNED_FOR_REVISION
  -> DRAFT

APPROVED
  -> ACTIVE

ACTIVE
  -> closing workflow

closing workflow
  -> CLOSED
```

Do not invent missing statuses or transitions.

## Roles

-   **PC:** create, revise, submit Advance Requests; record expenses;
    initiate closing.
-   **PM:** review, approve, or return assigned Advance Requests.
-   **Accounting:** confirm actual disbursement and review/finalize
    closing.
-   **MD:** paper signature outside the system in phase 1.

Role alone does not imply access to every Project. Verify relevant
Project relationships where required.

## Financial and workflow invariants

-   One Project must not have more than one `ACTIVE` Advance.
-   Enforce activation safely under concurrency.
-   Expenses are recorded while an Advance is `ACTIVE`.
-   OCR extraction is supporting evidence, not financial truth.
-   Use the user-confirmed Expense amount for Advance calculations.
-   OCR line items do not independently reduce the Advance balance.
-   Use exact decimal arithmetic.
-   Closing must satisfy:

``` text
Advance Amount = Total Confirmed Expenses + Return Amount
```

## Audit

Preserve enough history to identify:

-   actor
-   action
-   previous/resulting status
-   required reason
-   timestamp

## Do not guess

Stop and surface ambiguity when implementation depends on unresolved
behavior, including:

-   over-budget expenses
-   exact closing-return status
-   whether non-`ACTIVE` statuses block another Advance
-   fiscal-year blocking/override behavior
-   cancellation/rejection behavior
-   signed-document upload requirements
-   tax/VAT/WHT behavior

If the decision affects schema, API contracts, permissions, workflow, or
money, do not silently choose a rule.
