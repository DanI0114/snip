CREATE TABLE IF NOT EXISTS users (
    user_id     BIGSERIAL PRIMARY KEY,
    user_name   VARCHAR(...) NOT NULL,
    email       TEXT NOT NULL,
    password_hash   TEXT NOT NULL,
    user_status      VARCHAR(...) NOT NULL CHECK (status IN ('pending', 'active', 'disabled')) DEFAULT 'pending',
    user_role        VARCHAR (16) NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    email_verified_at   TIMESTAMPTZ,
    last_login_at       TIMESTAMPTZ,
    password_changed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_idx
ON users (LOWER(email));
