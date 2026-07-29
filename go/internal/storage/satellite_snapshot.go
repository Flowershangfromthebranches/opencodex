package storage

import (
	"context"
	"errors"
)

var errMissingLogsTable = errors.New("missing_logs_table")

// SnapshotLogsInLock captures the log rows belonging to the threads a cleanup
// is about to delete, so the delete can be undone.
//
// A missing logs table is an ERROR rather than an empty snapshot. Recording
// "nothing to restore" for a store we could not read would let the delete
// proceed with a rollback that silently restores nothing — the failure has to
// stop the cleanup instead.
func SnapshotLogsInLock(ctx context.Context, lock *SatelliteWriteLock, threadIDs []string) (map[string]any, error) {
	if lock == nil || len(threadIDs) == 0 {
		return nil, nil
	}
	exists, err := tableExistsOn(ctx, lock.conn, "logs")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errMissingLogsTable
	}
	rows := []SqlRow{}
	for _, chunk := range chunkIDs(threadIDs, sqliteIDChunk*2) {
		selected, err := selectRowsIn(ctx, lock.conn,
			`SELECT * FROM logs WHERE thread_id IN (`+placeholders(len(chunk))+`)`, anyArgs(chunk))
		if err != nil {
			return nil, err
		}
		rows = append(rows, selected...)
	}
	// `path` travels with the rows because a restore has to know which database
	// they came from; the backup file outlives the lock that produced it.
	return map[string]any{"path": lock.Path, "rows": rows}, nil
}

// SnapshotSatelliteBackupInLocks builds the backup document for whichever
// satellite stores are locked.
//
// Only locked sections are captured. A store that was never locked cannot be
// snapshotted consistently, and guessing its contents would produce a backup
// that restores the wrong thing.
func SnapshotSatelliteBackupInLocks(ctx context.Context, locks SatelliteWriteLocks, threadIDs []string) (map[string]any, error) {
	backup := map[string]any{"threadIds": threadIDs}
	if locks.Logs != nil {
		logs, err := SnapshotLogsInLock(ctx, locks.Logs, threadIDs)
		if err != nil {
			return nil, err
		}
		if logs != nil {
			backup["logs"] = logs
		}
	}
	return backup, nil
}
