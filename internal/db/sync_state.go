package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type SyncState struct {
	LastSyncAt       time.Time
	LastSyncCommit   string
	LastSyncDirty    bool
	IndexedFileCount int
	IndexFingerprint string
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type rowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func UpsertSyncState(ctx context.Context, ex execer, state SyncState) error {
	_, err := ex.ExecContext(ctx, `
INSERT INTO sync_state (
    id,
    last_sync_at,
    last_sync_commit,
    last_sync_dirty,
    indexed_file_count,
    index_fingerprint
) VALUES (1, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    last_sync_at = excluded.last_sync_at,
    last_sync_commit = excluded.last_sync_commit,
    last_sync_dirty = excluded.last_sync_dirty,
    indexed_file_count = excluded.indexed_file_count,
    index_fingerprint = excluded.index_fingerprint;
`, state.LastSyncAt.UTC().Format(time.RFC3339), state.LastSyncCommit, boolToInt(state.LastSyncDirty), state.IndexedFileCount, state.IndexFingerprint)
	if err != nil {
		return fmt.Errorf("upsert sync state: %w", err)
	}
	return nil
}

func LoadSyncState(ctx context.Context, q rowQueryer) (SyncState, bool, error) {
	var (
		state      SyncState
		timestamp  string
		dirtyInt   int
	)
	err := q.QueryRowContext(ctx, `
SELECT
    last_sync_at,
    last_sync_commit,
    last_sync_dirty,
    indexed_file_count,
    index_fingerprint
FROM sync_state
WHERE id = 1;
`).Scan(&timestamp, &state.LastSyncCommit, &dirtyInt, &state.IndexedFileCount, &state.IndexFingerprint)
	if err == sql.ErrNoRows {
		return SyncState{}, false, nil
	}
	if err != nil {
		return SyncState{}, false, fmt.Errorf("load sync state: %w", err)
	}

	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return SyncState{}, false, fmt.Errorf("parse sync timestamp: %w", err)
	}
	state.LastSyncAt = t
	state.LastSyncDirty = dirtyInt == 1
	return state, true, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
