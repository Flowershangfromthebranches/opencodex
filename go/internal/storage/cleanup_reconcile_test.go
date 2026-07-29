package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func reconcileHome(t *testing.T, stateRows, logRows string) string {
	t.Helper()
	home := t.TempDir()

	state, err := sql.Open("sqlite", "file:"+filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = state.Close() }()
	if _, err := state.Exec(`CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT, archived INTEGER, forked_from_id TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Exec(stateRows); err != nil {
		t.Fatal(err)
	}

	logs, err := sql.Open("sqlite", "file:"+filepath.Join(home, "logs_1.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = logs.Close() }()
	if _, err := logs.Exec(`CREATE TABLE logs (id TEXT PRIMARY KEY, thread_id TEXT, body TEXT)`); err != nil {
		t.Fatal(err)
	}
	if logRows != "" {
		if _, err := logs.Exec(logRows); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func stateThreadCount(t *testing.T, home string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM threads`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// The happy path: matched threads and their satellite rows go together.
func TestReconcileDeletesThreadsAndSatelliteRows(t *testing.T) {
	home := reconcileHome(t,
		`INSERT INTO threads VALUES ('doomed','archived_sessions/a.jsonl',1,NULL), ('keep','sessions/b.jsonl',0,NULL)`,
		`INSERT INTO logs VALUES ('l1','doomed','a'), ('l2','keep','b')`)

	result, err := ReconcileDeletedThreads(context.Background(),
		filepath.Join(home, "state.db"), DiscoverRuntimeDBPaths(home),
		[]ArchivedCandidate{{RelPath: "archived_sessions/a.jsonl"}}, home, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Threads) != 1 || result.Threads[0].ID != "doomed" {
		t.Fatalf("matched %#v", result.Threads)
	}
	if stateThreadCount(t, home) != 1 {
		t.Fatal("the doomed thread was not deleted")
	}
	if logRowCount(t, home) != 1 {
		t.Fatal("the doomed thread's logs were not deleted")
	}
}

// Referenced history refuses BEFORE any satellite row is touched, so a refused
// cleanup leaves the store exactly as it was.
func TestReconcileRefusesReferencedHistoryWithoutTouchingSatellites(t *testing.T) {
	home := reconcileHome(t,
		`INSERT INTO threads VALUES ('doomed','archived_sessions/a.jsonl',1,NULL), ('survivor','sessions/b.jsonl',0,'doomed')`,
		`INSERT INTO logs VALUES ('l1','doomed','a')`)

	result, err := ReconcileDeletedThreads(context.Background(),
		filepath.Join(home, "state.db"), DiscoverRuntimeDBPaths(home),
		[]ArchivedCandidate{{RelPath: "archived_sessions/a.jsonl"}}, home, 100)
	if !errors.Is(err, ErrReferencedHistory) {
		t.Fatalf("err = %v, want referenced_history", err)
	}
	if result.SatellitesMutated {
		t.Fatal("satellites were mutated for a cleanup that was refused")
	}
	if stateThreadCount(t, home) != 2 {
		t.Fatal("threads were deleted despite the refusal")
	}
	if logRowCount(t, home) != 1 {
		t.Fatal("log rows were deleted despite the refusal")
	}
}

// Nothing matched is not an error, and nothing is touched.
func TestReconcileWithNoMatchesIsANoOp(t *testing.T) {
	home := reconcileHome(t,
		`INSERT INTO threads VALUES ('keep','sessions/b.jsonl',0,NULL)`,
		`INSERT INTO logs VALUES ('l1','keep','b')`)

	result, err := ReconcileDeletedThreads(context.Background(),
		filepath.Join(home, "state.db"), DiscoverRuntimeDBPaths(home),
		[]ArchivedCandidate{{RelPath: "archived_sessions/missing.jsonl"}}, home, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Threads) != 0 {
		t.Fatalf("matched %#v with no candidates present", result.Threads)
	}
	if stateThreadCount(t, home) != 1 || logRowCount(t, home) != 1 {
		t.Fatal("a no-match reconcile changed the store")
	}
}

// A store with no state database has nothing to reconcile.
func TestReconcileWithoutAStateDatabase(t *testing.T) {
	if _, err := ReconcileDeletedThreads(context.Background(), "", RuntimeDBPaths{}, nil, t.TempDir(), 100); err != nil {
		t.Fatalf("missing state database was an error: %v", err)
	}
}
