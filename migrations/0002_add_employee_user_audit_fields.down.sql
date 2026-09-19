ALTER TABLE users
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS created_by;

ALTER TABLE employees
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS created_by;
