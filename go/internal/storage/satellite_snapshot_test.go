package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func logsHome(t *testing.T, seed func(*sql.DB)) string {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "logs_1.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	seed(db)
	return home
}

// The backup is what makes the delete undoable, so it has to capture exactly
// the rows belonging to the threads being removed -- no more, no less.
func TestSnapshotCapturesOnlyTheDoomedThreadsLogs(t *testing.T) {
	home := logsHome(t, func(db *sql.DB) {
		if _, err := db.Exec(`CREATE TABLE logs (id TEXT PRIMARY KEY, thread_id TEXT, body TEXT)`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO logs VALUES ('l1','doomed','a'), ('l2','doomed','b'), ('l3','keep','c')`); err != nil {
			t.Fatal(err)
		}
	})
	locks, err := BeginSatelliteWriteLocks(DiscoverRuntimeDBPaths(home), 100, AllSatellites())
	if err != nil {
		t.Fatal(err)
	}
	defer RollbackAllSatelliteLocks(&locks)

	backup, err := SnapshotSatelliteBackupInLocks(context.Background(), locks, []string{"doomed"})
	if err != nil {
		t.Fatal(err)
	}
	logs, present := backup["logs"].(map[string]any)
	if !present {
		t.Fatalf("no logs section captured: %#v", backup)
	}
	rows, _ := logs["rows"].([]SqlRow)
	if len(rows) != 2 {
		t.Fatalf("captured %d rows, want the two belonging to the doomed thread", len(rows))
	}
	// The path travels with the rows because a restore has to know which
	// database they came from; the backup file outlives this lock.
	if logs["path"] == "" || logs["path"] == nil {
		t.Fatalf("backup did not record its source database: %#v", logs)
	}
}

// A store we could not read must stop the cleanup. Recording "nothing to
// restore" would let the delete proceed behind a rollback that silently
// restores nothing.
func TestSnapshotRefusesAStoreWithoutALogsTable(t *testing.T) {
	home := logsHome(t, func(db *sql.DB) {
		if _, err := db.Exec(`CREATE TABLE unrelated (x)`); err != nil {
			t.Fatal(err)
		}
	})
	locks, err := BeginSatelliteWriteLocks(DiscoverRuntimeDBPaths(home), 100, AllSatellites())
	if err != nil {
		t.Fatal(err)
	}
	defer RollbackAllSatelliteLocks(&locks)

	if _, err := SnapshotSatelliteBackupInLocks(context.Background(), locks, []string{"doomed"}); !errors.Is(err, errMissingLogsTable) {
		t.Fatalf("err = %v, want the missing-table refusal", err)
	}
}

// Nothing to delete means nothing to back up, and the document still records
// the (empty) thread list a restore would check against.
func TestSnapshotWithNoThreadsCapturesNoSections(t *testing.T) {
	home := logsHome(t, func(db *sql.DB) {
		if _, err := db.Exec(`CREATE TABLE logs (id TEXT PRIMARY KEY, thread_id TEXT)`); err != nil {
			t.Fatal(err)
		}
	})
	locks, err := BeginSatelliteWriteLocks(DiscoverRuntimeDBPaths(home), 100, AllSatellites())
	if err != nil {
		t.Fatal(err)
	}
	defer RollbackAllSatelliteLocks(&locks)

	backup, err := SnapshotSatelliteBackupInLocks(context.Background(), locks, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := backup["logs"]; present {
		t.Fatalf("an empty delete captured a logs section: %#v", backup)
	}
	if _, present := backup["threadIds"]; !present {
		t.Fatalf("the thread list was omitted: %#v", backup)
	}
}
