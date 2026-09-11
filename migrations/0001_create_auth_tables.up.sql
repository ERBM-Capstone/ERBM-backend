-- Create roles table first (referenced by users)
CREATE TABLE roles (
    id         SERIAL       PRIMARY KEY,
    code       VARCHAR(50)  NOT NULL UNIQUE,
    name       VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);


-- Create employees table
CREATE TABLE employees (
    id                      BIGSERIAL    PRIMARY KEY,
    external_employee_id    VARCHAR(255) NOT NULL UNIQUE,
    
    email                   VARCHAR(255),
    first_name              VARCHAR(255) NOT NULL,
    last_name               VARCHAR(255) NOT NULL,
    first_name_en           VARCHAR(255),
    last_name_en            VARCHAR(255),
    nickname                VARCHAR(255),
    nickname_en             VARCHAR(255),
    mobile_no               VARCHAR(50),
    
    date_hired              DATE,
    position                VARCHAR(255),
    
    employment_status_id    INTEGER,
    employment_status       VARCHAR(100),
    is_active               BOOLEAN      NOT NULL DEFAULT true,
    
    organization_level1     VARCHAR(255),
    organization_level2     VARCHAR(255),
    organization_level3     VARCHAR(255),
    organization_level4     VARCHAR(255),
    organization_level5     VARCHAR(255),
    organization_level6     VARCHAR(255),
    
    raw_payload             JSONB,
    
    last_synced_at          TIMESTAMPTZ,
    created_at              TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Create indexes for employees
CREATE INDEX idx_employees_email ON employees (email);
CREATE INDEX idx_employees_is_active ON employees (is_active);

COMMENT ON COLUMN employees.external_employee_id IS 'empeo EmployeeRefId';
COMMENT ON COLUMN employees.employment_status_id IS 'raw empeo StatusId';
COMMENT ON COLUMN employees.employment_status IS 'raw empeo Status label';
COMMENT ON COLUMN employees.is_active IS 'interpreted active flag';
COMMENT ON COLUMN employees.raw_payload IS 'raw empeo response for debugging';

-- Create users table
CREATE TABLE users (
    id                      BIGSERIAL    PRIMARY KEY,
    employee_id             BIGINT       NOT NULL UNIQUE REFERENCES employees(id) ON DELETE CASCADE,
    
    email                   VARCHAR(255) NOT NULL UNIQUE,
    password_hash           VARCHAR(255),
    
    role_id                 INTEGER      NOT NULL DEFAULT 1 REFERENCES roles(id) ON DELETE RESTRICT,
    
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
