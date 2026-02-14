CREATE TABLE IF NOT EXISTS patterns (
    id          INTEGER PRIMARY KEY,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    confidence  TEXT NOT NULL DEFAULT 'medium',
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS pattern_files (
    id         INTEGER PRIMARY KEY,
    pattern_id INTEGER REFERENCES patterns(id) ON DELETE CASCADE,
    file_path  TEXT NOT NULL
);
