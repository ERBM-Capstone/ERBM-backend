-- Create roles table first (referenced by users)
CREATE TABLE roles (
    id         SERIAL       PRIMARY KEY,
    code       VARCHAR(50)  NOT NULL UNIQUE,
    name       VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Insert initial roles
INSERT INTO roles (code, name) VALUES
('EMPLOYEE', 'Employee'),
('ADMIN', 'Administrator'),
('PC', 'Project Coordinator'),
('PM', 'Project Manager'),
('ACCOUNTING', 'Accounting');



-- Create employees table
CREATE TABLE employees (
    id                  BIGSERIAL    PRIMARY KEY,
    first_name          VARCHAR(255) NOT NULL,
    last_name           VARCHAR(255) NOT NULL,
    position            VARCHAR(255) NOT NULL,
    email               VARCHAR(255) NOT NULL UNIQUE,
    date_hired          DATE         NOT NULL,
    employment_end_date DATE,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON COLUMN employees.employment_end_date IS
'null means currently employed; any value means inactive';

-- Create users table
CREATE TABLE users (
    id                      BIGSERIAL    PRIMARY KEY,
    employee_id             BIGINT       NOT NULL UNIQUE REFERENCES employees(id) ON DELETE CASCADE,
    
    email                   VARCHAR(255) NOT NULL UNIQUE,
    password_hash           VARCHAR(255),
    
    role_id                 INTEGER      NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    
    status                  VARCHAR(50)  NOT NULL DEFAULT 'INVITED',
    first_login             BOOLEAN      NOT NULL DEFAULT true,
    
    failed_login_attempts   INTEGER      NOT NULL DEFAULT 0,
    locked_until            TIMESTAMPTZ,
    
    last_login_at           TIMESTAMPTZ,
    created_at              TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Create indexes for users
CREATE INDEX idx_users_status ON users (status);

COMMENT ON COLUMN users.password_hash IS 'null until activated';
COMMENT ON COLUMN users.status IS 'INVITED, ACTIVE, DEACTIVATED';
