package storage

import (
	"context"
	"database/sql"
)

// DeleteThreadsAndDependents removes matched threads and everything that points
// at them.
//
// Order is load-bearing and matches the oracle (src/storage/cleanup.ts:668):
// dynamic tools, then spawn edges, then the threads themselves. Deleting the
// threads first would leave rows referencing ids that no longer exist, and a
// later reader cannot tell that from corruption.
//
// The caller owns the transaction. A partial delete is exactly the state this
// must never be left in, so committing is the caller's decision once every step
// has succeeded.
func DeleteThreadsAndDependents(ctx context.Context, conn *sql.Conn, threadIDs []string) error {
	if len(threadIDs) == 0 {
		return nil
	}
	hasDynamicTools, err := tableExistsOn(ctx, conn, "thread_dynamic_tools")
	if err != nil {
		return err
	}
	if hasDynamicTools {
		for _, chunk := range chunkIDs(threadIDs, sqliteIDChunk*2) {
			if _, err := conn.ExecContext(ctx,
				`DELETE FROM thread_dynamic_tools WHERE thread_id IN (`+placeholders(len(chunk))+`)`,
				anyArgs(chunk)...); err != nil {
				return err
			}
		}
	}

	hasEdges, err := tableExistsOn(ctx, conn, "thread_spawn_edges")
	if err != nil {
		return err
	}
	if hasEdges {
		for _, chunk := range chunkIDs(threadIDs, sqliteIDChunk) {
			arguments := append(anyArgs(chunk), anyArgs(chunk)...)
			if _, err := conn.ExecContext(ctx,
				`DELETE FROM thread_spawn_edges WHERE parent_thread_id IN (`+placeholders(len(chunk))+`) OR child_thread_id IN (`+placeholders(len(chunk))+`)`,
				arguments...); err != nil {
				return err
			}
		}
	}

	for _, chunk := range chunkIDs(threadIDs, sqliteIDChunk*2) {
		if _, err := conn.ExecContext(ctx,
			`DELETE FROM threads WHERE id IN (`+placeholders(len(chunk))+`)`,
			anyArgs(chunk)...); err != nil {
			return err
		}
	}
	return nil
}

func anyArgs(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
