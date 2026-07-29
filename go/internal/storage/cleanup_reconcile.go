package storage

import (
	"context"
	"database/sql"
	"errors"
)

// ErrReferencedHistory reports that deleting the matched threads would strand
// history something else still reaches.
var ErrReferencedHistory = errors.New("referenced_history")

// ReconcileResult reports what a reconcile did, including whether a failed
// attempt managed to put satellite rows back.
type ReconcileResult struct {
	Threads               []ThreadSnapshot
	Backup                map[string]any
	SatellitesMutated     bool
	SatelliteRestoreFaile bool
}

// ReconcileDeletedThreads performs the database half of an archived cleanup.
//
// Ordering is the whole design, and it follows the oracle
// (src/storage/cleanup.ts:1471-1520):
//
//  1. Take the state write lock and freeze the delete set under it. Anything
//     chosen before the lock could change underneath the delete.
//  2. Refuse if history is referenced — before touching a single satellite row.
//  3. Snapshot the satellites, then delete and commit them.
//  4. Re-check references under the SAME state lock. A concurrent writer can
//     fork a thread while the satellites were being mutated, and committing the
//     state delete then would strand it.
//  5. Delete the threads and commit.
//
// A failure after step 3 restores the satellite rows before returning, because
// those commits are already durable while the state delete is not.
func ReconcileDeletedThreads(
	ctx context.Context,
	statePath string,
	paths RuntimeDBPaths,
	candidates []ArchivedCandidate,
	codexHome string,
	busyTimeoutMS int,
) (ReconcileResult, error) {
	result := ReconcileResult{}
	if statePath == "" {
		return result, nil
	}
	db, err := sql.Open("sqlite", writableDSN(statePath, busyTimeoutMS))
	if err != nil {
		return result, err
	}
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return result, err
	}
	rollbackState := func() { _, _ = conn.ExecContext(ctx, "ROLLBACK") }

	threads, err := LoadMatchingThreads(ctx, conn, candidates, codexHome)
	if err != nil {
		rollbackState()
		return result, err
	}
	referenced, err := FindReferencedHistory(ctx, conn, threads)
	if err != nil {
		rollbackState()
		return result, err
	}
	if referenced {
		rollbackState()
		return result, ErrReferencedHistory
	}
	result.Threads = threads
	threadIDs := make([]string, 0, len(threads))
	for _, thread := range threads {
		threadIDs = append(threadIDs, thread.ID)
	}

	locks, err := BeginSatelliteWriteLocks(paths, busyTimeoutMS, AllSatellites())
	if err != nil {
		rollbackState()
		return result, err
	}
	backup, err := SnapshotSatelliteBackupInLocks(ctx, locks, threadIDs)
	if err != nil {
		RollbackAllSatelliteLocks(&locks)
		rollbackState()
		return result, err
	}
	result.Backup = backup

	_, hasSatelliteWork := backup["logs"]
	if hasSatelliteWork {
		result.SatellitesMutated = true
		if err := DeleteAndCommitSatellites(ctx, &locks, backup); err != nil {
			RollbackAllSatelliteLocks(&locks)
			rollbackState()
			result.SatelliteRestoreFaile = !RestoreSatelliteBackup(ctx, backup, busyTimeoutMS)
			return result, err
		}
	}
	RollbackAllSatelliteLocks(&locks)

	// Second check, same lock. The satellite work above released nothing on the
	// state database, but time passed: a concurrent writer can fork one of these
	// threads in that window, and committing the delete afterwards would strand
	// the fork.
	referenced, err = FindReferencedHistory(ctx, conn, threads)
	if err == nil && referenced {
		err = ErrReferencedHistory
	}
	if err != nil {
		rollbackState()
		if result.SatellitesMutated {
			result.SatelliteRestoreFaile = !RestoreSatelliteBackup(ctx, backup, busyTimeoutMS)
		}
		return result, err
	}

	if err := DeleteThreadsAndDependents(ctx, conn, threadIDs); err != nil {
		rollbackState()
		if result.SatellitesMutated {
			result.SatelliteRestoreFaile = !RestoreSatelliteBackup(ctx, backup, busyTimeoutMS)
		}
		return result, err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		rollbackState()
		if result.SatellitesMutated {
			result.SatelliteRestoreFaile = !RestoreSatelliteBackup(ctx, backup, busyTimeoutMS)
		}
		return result, err
	}
	return result, nil
}
