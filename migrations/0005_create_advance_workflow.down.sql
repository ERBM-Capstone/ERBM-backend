-- Schema rollback removes Advance records and their history; it does not
-- restore their data. Roll back dependent later migrations before this one.
DROP TABLE IF EXISTS advance_workflow_events;
DROP TABLE IF EXISTS advances;
