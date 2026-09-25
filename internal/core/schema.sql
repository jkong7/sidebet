CREATE TABLE IF NOT EXISTS users (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL,
    token_hash TEXT    NOT NULL UNIQUE,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS groups (
    id         INTEGER PRIMARY KEY,
    code       TEXT    NOT NULL UNIQUE,
    name       TEXT    NOT NULL,
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS members (
    group_id   INTEGER NOT NULL REFERENCES groups(id),
    user_id    INTEGER NOT NULL REFERENCES users(id),
    coins      REAL    NOT NULL,
    last_bonus INTEGER NOT NULL DEFAULT 0,
    joined_at  INTEGER NOT NULL,
    PRIMARY KEY (group_id, user_id)
);

CREATE TABLE IF NOT EXISTS markets (
    id          INTEGER PRIMARY KEY,
    group_id    INTEGER NOT NULL REFERENCES groups(id),
    creator_id  INTEGER NOT NULL REFERENCES users(id),
    subject_id  INTEGER REFERENCES users(id),
    question    TEXT    NOT NULL,
    b           REAL    NOT NULL,
    q_yes       REAL    NOT NULL DEFAULT 0,
    q_no        REAL    NOT NULL DEFAULT 0,
    status      TEXT    NOT NULL DEFAULT 'open',
    outcome     TEXT,
    closes_at   INTEGER NOT NULL,
    created_at  INTEGER NOT NULL,
    resolved_at INTEGER
);

CREATE INDEX IF NOT EXISTS markets_group ON markets (group_id, created_at DESC);

CREATE TABLE IF NOT EXISTS positions (
    market_id INTEGER NOT NULL REFERENCES markets(id),
    user_id   INTEGER NOT NULL REFERENCES users(id),
    yes       REAL    NOT NULL DEFAULT 0,
    no        REAL    NOT NULL DEFAULT 0,
    spent     REAL    NOT NULL DEFAULT 0,
    PRIMARY KEY (market_id, user_id)
);

CREATE TABLE IF NOT EXISTS trades (
    id          INTEGER PRIMARY KEY,
    market_id   INTEGER NOT NULL REFERENCES markets(id),
    user_id     INTEGER NOT NULL REFERENCES users(id),
    side        TEXT    NOT NULL,
    shares      REAL    NOT NULL,
    coins       REAL    NOT NULL,
    price_after REAL    NOT NULL,
    created_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS trades_market ON trades (market_id, id);
