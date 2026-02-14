ALTER TABLE symbol_deps RENAME TO symbol_deps_old;

CREATE TABLE symbol_deps (
    id          INTEGER PRIMARY KEY,
    symbol_id   INTEGER REFERENCES symbols(id) ON DELETE CASCADE,
    dep_name    TEXT NOT NULL,
    dep_package TEXT NOT NULL DEFAULT '',
    dep_kind    TEXT NOT NULL DEFAULT '',
    UNIQUE(symbol_id, dep_name, dep_package, dep_kind)
);

INSERT INTO symbol_deps (id, symbol_id, dep_name, dep_package, dep_kind)
SELECT id, symbol_id, dep_name, '', ''
FROM symbol_deps_old;

DROP TABLE symbol_deps_old;
