package storage

import (
	"context"
	"database/sql"
	"testing"
)

func deleteFixture(t *testing.T) *sql.Conn {
	t.Helper()
	conn, _ := threadsDB(t,
		`CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT);
		 CREATE TABLE thread_dynamic_tools (thread_id TEXT, name TEXT);
		 CREATE TABLE thread_spawn_edges (parent_thread_id TEXT, child_thread_id TEXT)`,
		`INSERT INTO threads VALUES ('doomed-a', 'archived_sessions/a.jsonl'), ('doomed-b', 'archived_sessions/b.jsonl'), ('keep', 'sessions/c.jsonl')`,
		`INSERT INTO thread_dynamic_tools VALUES ('doomed-a', 'tool-1'), ('keep', 'tool-2')`,
		`INSERT INTO thread_spawn_edges VALUES ('doomed-a', 'doomed-b'), ('keep', 'keep')`,
	)
	return conn
}

func countConnRows(t *testing.T, conn *sql.Conn, query string) int {
	t.Helper()
	var count int
	if err := conn.QueryRowContext(context.Background(), query).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// Everything pointing at a deleted thread goes with it. A leftover row
// referencing an id that no longer exists is indistinguishable from corruption
// to whatever reads the store next.
func TestDeleteRemovesThreadsAndEverythingPointingAtThem(t *testing.T) {
	conn := deleteFixture(t)
	ctx := context.Background()

	if err := DeleteThreadsAndDependents(ctx, conn, []string{"doomed-a", "doomed-b"}); err != nil {
		t.Fatal(err)
	}

	if count := countConnRows(t, conn, `SELECT COUNT(*) FROM threads`); count != 1 {
		t.Fatalf("threads remaining = %d, want only the untouched one", count)
	}
	if count := countConnRows(t, conn, `SELECT COUNT(*) FROM thread_dynamic_tools WHERE thread_id = 'doomed-a'`); count != 0 {
		t.Fatalf("dynamic tools survived their thread: %d", count)
	}
	if count := countConnRows(t, conn, `SELECT COUNT(*) FROM thread_spawn_edges WHERE parent_thread_id = 'doomed-a'`); count != 0 {
		t.Fatalf("spawn edges survived their thread: %d", count)
	}
}

// Rows belonging to threads outside the delete set are untouched, or a cleanup
// would quietly damage conversations it was never asked to remove.
func TestDeleteLeavesUnrelatedRowsAlone(t *testing.T) {
	conn := deleteFixture(t)
	if err := DeleteThreadsAndDependents(context.Background(), conn, []string{"doomed-a", "doomed-b"}); err != nil {
		t.Fatal(err)
	}
	if count := countConnRows(t, conn, `SELECT COUNT(*) FROM threads WHERE id = 'keep'`); count != 1 {
		t.Fatal("an unrelated thread was deleted")
	}
	if count := countConnRows(t, conn, `SELECT COUNT(*) FROM thread_dynamic_tools WHERE thread_id = 'keep'`); count != 1 {
		t.Fatal("an unrelated thread's dynamic tools were deleted")
	}
	if count := countConnRows(t, conn, `SELECT COUNT(*) FROM thread_spawn_edges WHERE parent_thread_id = 'keep'`); count != 1 {
		t.Fatal("an unrelated spawn edge was deleted")
	}
}

// A store whose schema predates the dependent tables still deletes cleanly
// rather than failing on a missing table.
func TestDeleteToleratesMissingDependentTables(t *testing.T) {
	conn, _ := threadsDB(t,
		`CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT)`,
		`INSERT INTO threads VALUES ('doomed', 'archived_sessions/a.jsonl')`,
	)
	if err := DeleteThreadsAndDependents(context.Background(), conn, []string{"doomed"}); err != nil {
		t.Fatalf("delete failed on a store without dependent tables: %v", err)
	}
	if count := countConnRows(t, conn, `SELECT COUNT(*) FROM threads`); count != 0 {
		t.Fatalf("thread survived: %d", count)
	}
}

// Nothing to delete is not an error, and must not touch the store.
func TestDeleteWithNoIDsIsANoOp(t *testing.T) {
	conn := deleteFixture(t)
	if err := DeleteThreadsAndDependents(context.Background(), conn, nil); err != nil {
		t.Fatal(err)
	}
	if count := countConnRows(t, conn, `SELECT COUNT(*) FROM threads`); count != 3 {
		t.Fatalf("an empty delete changed the store: %d threads remain", count)
	}
}
