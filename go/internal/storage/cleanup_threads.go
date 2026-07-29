package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
)

// ThreadSnapshot is one thread row a cleanup would delete.
type ThreadSnapshot struct {
	ID          string
	RolloutPath string
	Archived    *int64
	HistoryMode string
}

// NormalizeArchivedRolloutPath resolves a stored rollout path to its logical
// archived-relative form, or "" when the row does not name an archived rollout.
//
// This is the gate that keeps a cleanup inside archived_sessions: a row
// pointing at an active session, at another directory, or outside the home
// entirely resolves to "" and is therefore never matched. The oracle rejects
// the same three shapes (src/storage/cleanup.ts:221-240).
func NormalizeArchivedRolloutPath(rolloutPath, codexHome string) string {
	raw := strings.TrimSpace(filepath.ToSlash(rolloutPath))
	if raw == "" {
		return ""
	}
	home, err := filepath.Abs(codexHome)
	if err != nil {
		return ""
	}
	target := raw
	if !filepath.IsAbs(raw) && !windowsDriveAbsolute(raw) {
		target = filepath.Join(home, raw)
	}
	absolute, err := filepath.Abs(target)
	if err != nil {
		return ""
	}
	relative, err := filepath.Rel(home, absolute)
	if err != nil {
		return ""
	}
	logical := LogicalRolloutRelPath(filepath.ToSlash(relative))
	if logical == "" || strings.HasPrefix(logical, "..") {
		return ""
	}
	if !strings.HasPrefix(logical, ArchivedSessionsDir+"/") || !strings.HasSuffix(logical, jsonlSuffix) {
		return ""
	}
	return logical
}

// windowsDriveAbsolute recognises "C:\..." on every host, because a store
// written on Windows can be inspected elsewhere. A colon later in the name is
// not a drive letter — Codex rollout filenames carry ISO timestamps.
func windowsDriveAbsolute(path string) bool {
	return len(path) >= 3 && path[1] == ':' && (path[2] == '/' || path[2] == '\\')
}

// LoadMatchingThreads returns the thread rows whose rollout files are in the
// delete set.
//
// When the schema has an `archived` column, only archived=1 rows are eligible:
// an active thread whose rollout happens to sit under archived_sessions is not
// something a cleanup may remove. Older schemas without the column fall back to
// path matching alone, which is what the oracle does.
func LoadMatchingThreads(ctx context.Context, conn *sql.Conn, candidates []ArchivedCandidate, codexHome string) ([]ThreadSnapshot, error) {
	exists, err := tableExistsOn(ctx, conn, "threads")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errMissingThreadsTable
	}
	columns, err := tableColumnNames(ctx, conn, "threads")
	if err != nil {
		return nil, err
	}
	wanted := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		wanted[candidate.RelPath] = true
	}
	selected := []string{"id", "rollout_path"}
	if columns["archived"] {
		selected = append(selected, "archived")
	}
	if columns["history_mode"] {
		selected = append(selected, "history_mode")
	}
	rows, err := conn.QueryContext(ctx, `SELECT `+strings.Join(selected, ", ")+` FROM threads`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var matched []ThreadSnapshot
	for rows.Next() {
		var (
			id          string
			rolloutPath sql.NullString
			archived    sql.NullInt64
			historyMode sql.NullString
		)
		targets := []any{&id, &rolloutPath}
		if columns["archived"] {
			targets = append(targets, &archived)
		}
		if columns["history_mode"] {
			targets = append(targets, &historyMode)
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, err
		}
		if columns["archived"] && archived.Int64 != 1 {
			continue
		}
		normalized := NormalizeArchivedRolloutPath(rolloutPath.String, codexHome)
		if normalized == "" || !wanted[normalized] {
			continue
		}
		snapshot := ThreadSnapshot{ID: id, RolloutPath: rolloutPath.String, HistoryMode: historyMode.String}
		if columns["archived"] && archived.Valid {
			value := archived.Int64
			snapshot.Archived = &value
		}
		matched = append(matched, snapshot)
	}
	return matched, rows.Err()
}

// FindReferencedHistory reports whether deleting these threads would break
// history something else still reaches.
//
// Three ways that happens, and any one of them refuses the cleanup:
//   - paginated history keeps durable projections tied to the rollout;
//   - a spawn edge crossing the delete boundary leaves a parent or child
//     pointing at a thread that no longer exists;
//   - a surviving thread names one of ours as its fork/parent source.
//
// Real database errors propagate rather than being read as "no references":
// treating a busy or corrupt store as clearance would delete data on the
// strength of a failed question.
func FindReferencedHistory(ctx context.Context, conn *sql.Conn, threads []ThreadSnapshot) (bool, error) {
	if len(threads) == 0 {
		return false, nil
	}
	ids := make([]string, 0, len(threads))
	inSet := make(map[string]bool, len(threads))
	for _, thread := range threads {
		ids = append(ids, thread.ID)
		inSet[thread.ID] = true
		if strings.EqualFold(strings.TrimSpace(thread.HistoryMode), "paginated") {
			return true, nil
		}
	}

	hasEdges, err := tableExistsOn(ctx, conn, "thread_spawn_edges")
	if err != nil {
		return false, err
	}
	if hasEdges {
		for _, chunk := range chunkIDs(ids, sqliteIDChunk) {
			placeholders := placeholders(len(chunk))
			arguments := make([]any, 0, len(chunk)*2)
			for range 2 {
				for _, id := range chunk {
					arguments = append(arguments, id)
				}
			}
			rows, err := conn.QueryContext(ctx,
				`SELECT parent_thread_id, child_thread_id FROM thread_spawn_edges
				 WHERE parent_thread_id IN (`+placeholders+`) OR child_thread_id IN (`+placeholders+`)`,
				arguments...)
			if err != nil {
				return false, err
			}
			crosses, scanErr := edgeCrossesBoundary(rows, inSet)
			_ = rows.Close()
			if scanErr != nil {
				return false, scanErr
			}
			if crosses {
				return true, nil
			}
		}
	}

	columns, err := tableColumnNames(ctx, conn, "threads")
	if err != nil {
		return false, err
	}
	for _, column := range []string{"forked_from_id", "parent_thread_id", "source_thread_id"} {
		if !columns[column] {
			continue
		}
		for _, chunk := range chunkIDs(ids, sqliteIDChunk*2) {
			arguments := make([]any, 0, len(chunk))
			for _, id := range chunk {
				arguments = append(arguments, id)
			}
			rows, err := conn.QueryContext(ctx,
				`SELECT id FROM threads WHERE `+quoteIdent(column)+` IN (`+placeholders(len(chunk))+`)`,
				arguments...)
			if err != nil {
				return false, err
			}
			outside, scanErr := anyIDOutside(rows, inSet)
			_ = rows.Close()
			if scanErr != nil {
				return false, scanErr
			}
			if outside {
				return true, nil
			}
		}
	}
	return false, nil
}

func edgeCrossesBoundary(rows *sql.Rows, inSet map[string]bool) (bool, error) {
	for rows.Next() {
		var parent, child sql.NullString
		if err := rows.Scan(&parent, &child); err != nil {
			return false, err
		}
		if !inSet[parent.String] || !inSet[child.String] {
			return true, nil
		}
	}
	return false, rows.Err()
}

func anyIDOutside(rows *sql.Rows, inSet map[string]bool) (bool, error) {
	for rows.Next() {
		var id sql.NullString
		if err := rows.Scan(&id); err != nil {
			return false, err
		}
		if !inSet[id.String] {
			return true, nil
		}
	}
	return false, rows.Err()
}


