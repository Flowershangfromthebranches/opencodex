package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func threadsDB(t *testing.T, schema string, statements ...string) (*sql.Conn, string) {
	t.Helper()
	home := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, home
}

func candidates(relPaths ...string) []ArchivedCandidate {
	out := make([]ArchivedCandidate, 0, len(relPaths))
	for _, relPath := range relPaths {
		out = append(out, ArchivedCandidate{RelPath: relPath})
	}
	return out
}

// A cleanup may only ever touch archived_sessions. A thread whose rollout points
// at an active session, at another directory, or outside the home entirely must
// not match — otherwise a percentage cleanup could delete live conversations.
func TestNormalizeArchivedRolloutPathRejectsEverythingElse(t *testing.T) {
	home := "/tmp/codex-home"
	for _, testCase := range []struct{ name, path, want string }{
		{"archived relative", "archived_sessions/2026/rollout-a.jsonl", "archived_sessions/2026/rollout-a.jsonl"},
		{"archived absolute", home + "/archived_sessions/2026/rollout-a.jsonl", "archived_sessions/2026/rollout-a.jsonl"},
		{"compressed archived", "archived_sessions/2026/rollout-a.jsonl.zst", "archived_sessions/2026/rollout-a.jsonl"},
		{"active session", "sessions/2026/rollout-a.jsonl", ""},
		{"outside the home", "/etc/passwd", ""},
		{"escaping traversal", "archived_sessions/../../etc/passwd", ""},
		{"not a rollout", "archived_sessions/2026/notes.txt", ""},
		{"blank", "   ", ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := NormalizeArchivedRolloutPath(testCase.path, home); got != testCase.want {
				t.Fatalf("normalize(%q) = %q, want %q", testCase.path, got, testCase.want)
			}
		})
	}
}

// Only archived rows are eligible when the schema says which are archived. An
// active thread whose rollout happens to sit under archived_sessions is not
// something a cleanup may remove.
func TestLoadMatchingThreadsHonorsTheArchivedColumn(t *testing.T) {
	conn, home := threadsDB(t,
		`CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT, archived INTEGER, history_mode TEXT)`,
		`INSERT INTO threads VALUES ('archived-1', 'archived_sessions/a.jsonl', 1, NULL)`,
		`INSERT INTO threads VALUES ('active-1', 'archived_sessions/b.jsonl', 0, NULL)`,
		`INSERT INTO threads VALUES ('elsewhere', 'sessions/c.jsonl', 1, NULL)`,
	)

	threads, err := LoadMatchingThreads(context.Background(), conn, candidates(
		"archived_sessions/a.jsonl", "archived_sessions/b.jsonl", "sessions/c.jsonl",
	), home)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || threads[0].ID != "archived-1" {
		t.Fatalf("matched %#v, want only the archived row inside archived_sessions", threads)
	}
}

// A schema without the column predates the flag, so path matching alone
// decides — the same fallback the oracle takes.
func TestLoadMatchingThreadsWithoutAnArchivedColumn(t *testing.T) {
	conn, home := threadsDB(t,
		`CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT)`,
		`INSERT INTO threads VALUES ('one', 'archived_sessions/a.jsonl')`,
	)
	threads, err := LoadMatchingThreads(context.Background(), conn, candidates("archived_sessions/a.jsonl"), home)
	if err != nil || len(threads) != 1 {
		t.Fatalf("threads=%#v err=%v", threads, err)
	}
}

// A missing threads table is refused rather than read as "nothing matched",
// which would let a cleanup proceed against a store it does not understand.
func TestLoadMatchingThreadsRefusesAMissingTable(t *testing.T) {
	conn, home := threadsDB(t, `CREATE TABLE unrelated (id TEXT)`)
	if _, err := LoadMatchingThreads(context.Background(), conn, candidates("archived_sessions/a.jsonl"), home); err == nil {
		t.Fatal("a missing threads table was accepted")
	}
}

// Deleting a thread that something else still reaches would strand the survivor,
// so each of these refuses the cleanup.
func TestFindReferencedHistoryDetectsEachReference(t *testing.T) {
	t.Run("paginated history", func(t *testing.T) {
		conn, _ := threadsDB(t, `CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT)`)
		referenced, err := FindReferencedHistory(context.Background(), conn, []ThreadSnapshot{{ID: "a", HistoryMode: "Paginated"}})
		if err != nil || !referenced {
			t.Fatalf("referenced=%v err=%v", referenced, err)
		}
	})

	t.Run("spawn edge crossing the boundary", func(t *testing.T) {
		conn, _ := threadsDB(t,
			`CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT);
			 CREATE TABLE thread_spawn_edges (parent_thread_id TEXT, child_thread_id TEXT)`,
			`INSERT INTO thread_spawn_edges VALUES ('doomed', 'survivor')`,
		)
		referenced, err := FindReferencedHistory(context.Background(), conn, []ThreadSnapshot{{ID: "doomed"}})
		if err != nil || !referenced {
			t.Fatalf("referenced=%v err=%v", referenced, err)
		}
	})

	t.Run("surviving fork source", func(t *testing.T) {
		conn, _ := threadsDB(t,
			`CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT, forked_from_id TEXT)`,
			`INSERT INTO threads VALUES ('survivor', 'sessions/s.jsonl', 'doomed')`,
		)
		referenced, err := FindReferencedHistory(context.Background(), conn, []ThreadSnapshot{{ID: "doomed"}})
		if err != nil || !referenced {
			t.Fatalf("referenced=%v err=%v", referenced, err)
		}
	})
}

// An edge entirely inside the delete set strands nothing, so the cleanup may
// proceed — a rule that refuses everything would make the feature useless.
func TestFindReferencedHistoryAllowsSelfContainedDeletes(t *testing.T) {
	conn, _ := threadsDB(t,
		`CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT, forked_from_id TEXT);
		 CREATE TABLE thread_spawn_edges (parent_thread_id TEXT, child_thread_id TEXT)`,
		`INSERT INTO thread_spawn_edges VALUES ('a', 'b')`,
	)
	referenced, err := FindReferencedHistory(context.Background(), conn, []ThreadSnapshot{{ID: "a"}, {ID: "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if referenced {
		t.Fatal("a delete set containing both ends of the edge was refused")
	}
}

func TestFindReferencedHistoryIsEmptyForNoThreads(t *testing.T) {
	conn, _ := threadsDB(t, `CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT)`)
	if referenced, err := FindReferencedHistory(context.Background(), conn, nil); err != nil || referenced {
		t.Fatalf("referenced=%v err=%v", referenced, err)
	}
}
