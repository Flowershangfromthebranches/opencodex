package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// DeleteLogsInLock removes the exact rows the backup captured.
//
// It deletes by row id rather than by thread, so a log written between the
// snapshot and the delete survives: the backup can only restore what it saw,
// and deleting more than that would be unrecoverable.
func DeleteLogsInLock(ctx context.Context, lock *SatelliteWriteLock, rows []SqlRow) error {
	if lock == nil || len(rows) == 0 {
		return nil
	}
	exists, err := tableExistsOn(ctx, lock.conn, "logs")
	if err != nil {
		return err
	}
	if !exists {
		return errMissingLogsTable
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if id, ok := row["id"]; ok && id != nil {
			ids = append(ids, sqlTextValue(id))
		}
	}
	if len(ids) == 0 {
		return nil
	}
	for _, chunk := range chunkIDs(ids, sqliteIDChunk*2) {
		if _, err := lock.conn.ExecContext(ctx,
			`DELETE FROM logs WHERE id IN (`+placeholders(len(chunk))+`)`, anyArgs(chunk)...); err != nil {
			return err
		}
	}
	return nil
}

// DeleteAndCommitSatellites applies the satellite deletes and commits each lock
// as it completes.
//
// Committing per store rather than at the end is deliberate: the locks are
// separate databases and cannot be committed atomically, so the sequence is
// delete-then-commit-then-release, and the caller restores from the backup if a
// later store fails. Holding every lock until the end would widen the window
// where an unrelated writer is blocked without making the operation any more
// atomic than it already is not.
func DeleteAndCommitSatellites(ctx context.Context, locks *SatelliteWriteLocks, backup map[string]any) error {
	if locks == nil {
		return nil
	}
	if logs, ok := backup["logs"].(map[string]any); ok && locks.Logs != nil {
		rows, _ := logs["rows"].([]SqlRow)
		if err := DeleteLogsInLock(ctx, locks.Logs, rows); err != nil {
			return err
		}
		if err := CommitSatelliteLock(locks.Logs); err != nil {
			return err
		}
		locks.Logs = nil
	}
	return nil
}

// RestoreSatelliteBackup puts the captured rows back.
//
// Inserts ignore conflicts because a restore may run after a partial delete, so
// some rows are still present; failing on those would abandon the rest of the
// restore. Reports success so the caller can tell the user their data is back
// rather than silently leaving a half-cleaned store.
func RestoreSatelliteBackup(ctx context.Context, backup map[string]any, busyTimeoutMS int) bool {
	logs, ok := backup["logs"].(map[string]any)
	if !ok {
		return true
	}
	path, _ := logs["path"].(string)
	rows, _ := logs["rows"].([]SqlRow)
	if path == "" || len(rows) == 0 {
		return true
	}
	db, err := sql.Open("sqlite", writableDSN(path, busyTimeoutMS))
	if err != nil {
		return false
	}
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(ctx)
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()

	exists, err := tableExistsOn(ctx, conn, "logs")
	if err != nil || !exists {
		return false
	}
	if _, err := InsertRowsConflictIgnore(ctx, conn, "logs", rows); err != nil {
		return false
	}
	return true
}

// sqlTextValue renders a scanned id for use as a bind parameter. Ids arrive as
// string or []byte depending on the column type, and a %v of []byte would bind
// the Go slice syntax rather than the id.
func sqlTextValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}
