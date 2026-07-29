package oauth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func legacyStoreFile(t *testing.T, body string) (*CredentialStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return NewCredentialStore(path), path
}

const legacyAuthJSON = `{"anthropic":{"access":"legacy-access","refresh":"legacy-refresh","expires":4102444800000}}`

// An older build reading the new multi-account shape drops what it cannot parse
// and then persists the remainder, destroying refresh tokens. The first write
// over a legacy file therefore keeps a pristine copy, so that downgrade is
// recoverable instead of costing the user every provider login
// (oracle: src/oauth/store.ts:169-183).
func TestFirstWriteOverLegacyStoreKeepsABackup(t *testing.T) {
	store, path := legacyStoreFile(t, legacyAuthJSON)

	credential := OAuthCredentials{Access: "new-access", Refresh: "new-refresh", Expires: time.Now().Add(time.Hour).UnixMilli()}
	if err := store.SaveCredential(context.Background(), "anthropic", credential); err != nil {
		t.Fatal(err)
	}

	backup, err := os.ReadFile(path + ".pre-multiauth")
	if err != nil {
		t.Fatalf("no pre-multiauth backup was written: %v", err)
	}
	if string(backup) != legacyAuthJSON {
		t.Fatalf("backup is not the pristine legacy file: %s", backup)
	}
	var current map[string]any
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &current); err != nil {
		t.Fatalf("live store is not valid JSON: %v", err)
	}
	if _, present := current["anthropic"]; !present {
		t.Fatal("the credential write itself did not land")
	}
}

// Once, not on every write: a later mutation must not overwrite the pristine
// copy with an already-migrated file, which would defeat the whole point.
func TestBackupIsWrittenOnlyOnce(t *testing.T) {
	store, path := legacyStoreFile(t, legacyAuthJSON)
	ctx := context.Background()
	credential := OAuthCredentials{Access: "a", Refresh: "r", Expires: time.Now().Add(time.Hour).UnixMilli()}

	if err := store.SaveCredential(ctx, "anthropic", credential); err != nil {
		t.Fatal(err)
	}
	credential.Access = "b"
	if err := store.SaveCredential(ctx, "anthropic", credential); err != nil {
		t.Fatal(err)
	}

	backup, err := os.ReadFile(path + ".pre-multiauth")
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != legacyAuthJSON {
		t.Fatalf("a later write overwrote the pristine backup: %s", backup)
	}
}

// A store already in the new shape has nothing to preserve, so no stray file
// appears next to the user's credentials.
func TestNoBackupForAnAlreadyMigratedStore(t *testing.T) {
	store, path := legacyStoreFile(t, `{"anthropic":{"activeAccountId":"one","accounts":[{"id":"one","credential":{"access":"a","refresh":"r","expires":4102444800000}}]}}`)

	credential := OAuthCredentials{Access: "b", Refresh: "r", Expires: time.Now().Add(time.Hour).UnixMilli()}
	if err := store.SaveCredential(context.Background(), "anthropic", credential); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path + ".pre-multiauth"); err == nil {
		t.Fatal("a backup was written for a store that was never legacy")
	}
}

// The backup carries credentials, so it must not be world-readable.
func TestBackupIsNotWorldReadable(t *testing.T) {
	store, path := legacyStoreFile(t, legacyAuthJSON)
	credential := OAuthCredentials{Access: "a", Refresh: "r", Expires: time.Now().Add(time.Hour).UnixMilli()}
	if err := store.SaveCredential(context.Background(), "anthropic", credential); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path + ".pre-multiauth")
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Fatalf("backup mode = %v, credentials must stay owner-only", mode)
	}
}
