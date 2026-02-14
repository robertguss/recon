ALTER TABLE symbol_deps RENAME TO symbol_deps_new;

CREATE TABLE symbol_deps (
    id        INTEGER PRIMARY KEY,
    symbol_id INTEGER REFERENCES symbols(id) ON DELETE CASCADE,
    dep_name  TEXT NOT NULL,
    UNIQUE(symbol_id, dep_name)
);

INSERT OR IGNORE INTO symbol_deps (id, symbol_id, dep_name)
SELECT id, symbol_id, dep_name
FROM symbol_deps_new;

DROP TABLE symbol_deps_new;
