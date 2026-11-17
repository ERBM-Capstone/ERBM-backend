-- Stop receipt/expense endpoints and drain or export pending deletion tasks first.
-- This drops data; reapplying UP does not restore records or deleted file bytes.
-- Roll back OCR/Closing dependencies before this version.
DROP TABLE IF EXISTS expense_documents;
DROP TABLE IF EXISTS expense_audit_events;
DROP TABLE IF EXISTS file_deletion_tasks;
DROP TABLE IF EXISTS expenses;
