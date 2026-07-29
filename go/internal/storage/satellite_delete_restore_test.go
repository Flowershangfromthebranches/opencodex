package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func logsStore(t *testing.T, rows string) string {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "logs_1.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE logs (id TEXT PRIMARY KEY, thread_id TEXT, body TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(rows); err != nil {
		t.Fatal(err)
	}
	return home
}

func logRowCount(t *testing.T, home string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(home, "logs_1.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM logs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// The whole point of the backup: after a delete, restoring puts the rows back.
// This is the rollback proof the coordinator will depend on.
func TestSatelliteDeleteThenRestoreRoundTrips(t *testing.T) {
	home := logsStore(t, `INSERT INTO logs VALUES ('l1','doomed','a'), ('l2','doomed','b'), ('l3','keep','c')`)
	ctx := context.Background()

	locks, err := BeginSatelliteWriteLocks(DiscoverRuntimeDBPaths(home), 100, AllSatellites())
	if err != nil {
		t.Fatal(err)
	}
	backup, err := SnapshotSatelliteBackupInLocks(ctx, locks, []string{"doomed"})
	if err != nil {
		RollbackAllSatelliteLocks(&locks)
		t.Fatal(err)
	}
	if err := DeleteAndCommitSatellites(ctx, &locks, backup); err != nil {
		RollbackAllSatelliteLocks(&locks)
		t.Fatal(err)
	}
	RollbackAllSatelliteLocks(&locks)

	if count := logRowCount(t, home); count != 1 {
		t.Fatalf("after delete %d rows remain, want only the untouched one", count)
	}
	if !RestoreSatelliteBackup(ctx, backup, 100) {
		t.Fatal("restore reported failure")
	}
	if count := logRowCount(t, home); count != 3 {
		t.Fatalf("after restore %d rows, want the original three back", count)
	}
}

// A restore may run after a PARTIAL delete, so some rows are still present.
// Failing on those would abandon the rest of the restore and leave the store
// half-cleaned.
func TestRestoreIgnoresRowsThatSurvived(t *testing.T) {
	home := logsStore(t, `INSERT INTO logs VALUES ('l1','doomed','a')`)
	backup := map[string]any{
		"threadIds": []string{"doomed"},
		"logs": map[string]any{
			"path": filepath.Join(home, "logs_1.sqlite"),
			"rows": []SqlRow{{"id": "l1", "thread_id": "doomed", "body": "a"}},
		},
	}
	if !RestoreSatelliteBackup(context.Background(), backup, 100) {
		t.Fatal("restore failed on a row that was still present")
	}
	if count := logRowCount(t, home); count != 1 {
		t.Fatalf("restore duplicated a surviving row: %d", count)
	}
}

// Deleting by captured row id rather than by thread means a log written between
// the snapshot and the delete survives -- the backup can only restore what it
// saw, so deleting more than that would be unrecoverable.
func TestDeleteOnlyRemovesRowsTheBackupCaptured(t *testing.T) {
	home := logsStore(t, `INSERT INTO logs VALUES ('l1','doomed','a')`)
	ctx := context.Background()

	locks, err := BeginSatelliteWriteLocks(DiscoverRuntimeDBPaths(home), 100, AllSatellites())
	if err != nil {
		t.Fatal(err)
	}
	backup, err := SnapshotSatelliteBackupInLocks(ctx, locks, []string{"doomed"})
	if err != nil {
		RollbackAllSatelliteLocks(&locks)
		t.Fatal(err)
	}
	// A row that arrives after the snapshot, on the same thread.
	if _, err := locks.Logs.conn.ExecContext(ctx, `INSERT INTO logs VALUES ('l2','doomed','later')`); err != nil {
		RollbackAllSatelliteLocks(&locks)
		t.Fatal(err)
	}
	if err := DeleteAndCommitSatellites(ctx, &locks, backup); err != nil {
		RollbackAllSatelliteLocks(&locks)
		t.Fatal(err)
	}
	RollbackAllSatelliteLocks(&locks)

	if count := logRowCount(t, home); count != 1 {
		t.Fatalf("%d rows remain; the post-snapshot row should have survived", count)
	}
}

// Nothing captured means nothing to do, and the store is untouched.
func TestDeleteAndRestoreAreNoOpsForAnEmptyBackup(t *testing.T) {
	home := logsStore(t, `INSERT INTO logs VALUES ('l1','keep','a')`)
	ctx := context.Background()
	locks, err := BeginSatelliteWriteLocks(DiscoverRuntimeDBPaths(home), 100, AllSatellites())
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteAndCommitSatellites(ctx, &locks, map[string]any{"threadIds": []string{}}); err != nil {
		t.Fatal(err)
	}
	RollbackAllSatelliteLocks(&locks)

	if !RestoreSatelliteBackup(ctx, map[string]any{}, 100) {
		t.Fatal("an empty restore reported failure")
	}
	if count := logRowCount(t, home); count != 1 {
		t.Fatalf("an empty backup changed the store: %d", count)
	}
}
