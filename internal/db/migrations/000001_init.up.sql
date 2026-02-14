CREATE TABLE IF NOT EXISTS packages (
    id          INTEGER PRIMARY KEY,
    path        TEXT UNIQUE NOT NULL,
    name        TEXT NOT NULL,
    import_path TEXT,
    file_count  INTEGER DEFAULT 0,
    line_count  INTEGER DEFAULT 0,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS files (
    id          INTEGER PRIMARY KEY,
    package_id  INTEGER REFERENCES packages(id),
    path        TEXT UNIQUE NOT NULL,
    language    TEXT NOT NULL DEFAULT 'go',
    lines       INTEGER NOT NULL,
    hash        TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS symbols (
    id          INTEGER PRIMARY KEY,
    file_id     INTEGER REFERENCES files(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    name        TEXT NOT NULL,
    signature   TEXT,
    body        TEXT,
    line_start  INTEGER NOT NULL,
    line_end    INTEGER NOT NULL,
    exported    INTEGER NOT NULL,
    receiver    TEXT NOT NULL DEFAULT '',
    UNIQUE(file_id, kind, name, receiver)
);

CREATE TABLE IF NOT EXISTS imports (
    id            INTEGER PRIMARY KEY,
    from_file_id  INTEGER REFERENCES files(id) ON DELETE CASCADE,
    to_path       TEXT NOT NULL,
    to_package_id INTEGER REFERENCES packages(id),
    alias         TEXT,
    import_type   TEXT NOT NULL,
    UNIQUE(from_file_id, to_path)
);

CREATE TABLE IF NOT EXISTS symbol_deps (
    id       INTEGER PRIMARY KEY,
    symbol_id INTEGER REFERENCES symbols(id) ON DELETE CASCADE,
    dep_name TEXT NOT NULL,
    dep_package TEXT NOT NULL DEFAULT '',
    dep_kind TEXT NOT NULL DEFAULT '',
    UNIQUE(symbol_id, dep_name, dep_package, dep_kind)
);

CREATE TABLE IF NOT EXISTS decisions (
    id          INTEGER PRIMARY KEY,
    title       TEXT NOT NULL,
    reasoning   TEXT NOT NULL,
    confidence  TEXT NOT NULL DEFAULT 'medium',
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS evidence (
    id               INTEGER PRIMARY KEY,
    entity_type      TEXT NOT NULL,
    entity_id        INTEGER NOT NULL,
    summary          TEXT NOT NULL,
    check_type       TEXT,
    check_spec       TEXT,
    baseline         TEXT,
    last_verified_at TEXT,
    last_result      TEXT,
    drift_status     TEXT DEFAULT 'ok'
);

CREATE TABLE IF NOT EXISTS proposals (
    id          INTEGER PRIMARY KEY,
    session_id  INTEGER REFERENCES sessions(id),
    entity_type TEXT NOT NULL,
    entity_data TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    proposed_at TEXT NOT NULL,
    verified_at TEXT,
    promoted_at TEXT
);

CREATE TABLE IF NOT EXISTS sessions (
    id         INTEGER PRIMARY KEY,
    started_at TEXT NOT NULL,
    ended_at   TEXT,
    summary    TEXT
);

CREATE TABLE IF NOT EXISTS session_files (
    session_id INTEGER REFERENCES sessions(id) ON DELETE CASCADE,
    file_id    INTEGER REFERENCES files(id) ON DELETE CASCADE,
    PRIMARY KEY (session_id, file_id)
);

CREATE TABLE IF NOT EXISTS sync_state (
    id                 INTEGER PRIMARY KEY CHECK (id = 1),
    last_sync_at       TEXT NOT NULL,
    last_sync_commit   TEXT,
    last_sync_dirty    INTEGER NOT NULL DEFAULT 0,
    indexed_file_count INTEGER NOT NULL DEFAULT 0,
    index_fingerprint  TEXT NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS search_index USING fts5 (
    title,
    content,
    entity_type UNINDEXED,
    entity_id UNINDEXED,
    tokenize='porter'
);
