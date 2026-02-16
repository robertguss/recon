CREATE TABLE IF NOT EXISTS pattern_files (
    id         INTEGER PRIMARY KEY,
    pattern_id INTEGER REFERENCES patterns(id) ON DELETE CASCADE,
    file_path  TEXT NOT NULL
);

INSERT INTO pattern_files (pattern_id, file_path)
SELECT from_id, to_ref FROM edges
WHERE from_type = 'pattern' AND to_type = 'file' AND relation = 'affects';

DROP TABLE IF EXISTS edges;
