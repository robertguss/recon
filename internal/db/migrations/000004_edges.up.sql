CREATE TABLE IF NOT EXISTS edges (
    id          INTEGER PRIMARY KEY,
    from_type   TEXT NOT NULL,
    from_id     INTEGER NOT NULL,
    to_type     TEXT NOT NULL,
    to_ref      TEXT NOT NULL,
    relation    TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT 'manual',
    confidence  TEXT NOT NULL DEFAULT 'medium',
    created_at  TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_edges_unique
    ON edges(from_type, from_id, to_type, to_ref, relation);
CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_type, from_id);
CREATE INDEX IF NOT EXISTS idx_edges_to ON edges(to_type, to_ref);
CREATE INDEX IF NOT EXISTS idx_edges_relation ON edges(relation);

-- Migrate pattern_files into edges
INSERT INTO edges (from_type, from_id, to_type, to_ref, relation, source, confidence, created_at)
SELECT 'pattern', pf.pattern_id, 'file', pf.file_path, 'affects', 'auto', 'medium',
       COALESCE(p.created_at, datetime('now'))
FROM pattern_files pf
LEFT JOIN patterns p ON p.id = pf.pattern_id;

DROP TABLE IF EXISTS pattern_files;
