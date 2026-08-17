CREATE TABLE IF NOT EXISTS users (
    user_id             BIGSERIAL PRIMARY KEY,
    name                VARCHAR(64) NOT NULL,
    email               TEXT NOT NULL,
    password_hash       TEXT NOT NULL,

    status              VARCHAR(16) NOT NULL
                        DEFAULT 'pending'
                        CHECK (status IN ('pending', 'active', 'disabled')),

    role                VARCHAR(16) NOT NULL
                        DEFAULT 'user'
                        CHECK (role IN ('user', 'admin')),

    email_verified_at   TIMESTAMPTZ,
    last_login_at       TIMESTAMPTZ,
    password_changed_at TIMESTAMPTZ,

    created_at          TIMESTAMPTZ NOT NULL
                        DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_idx
ON users (LOWER(email));
