package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// After an unclean exit the pid file survives, and the OS is free to hand that
// number to an unrelated program. Refusing to start on liveness alone would
// lock the user out until they deleted the file by hand, so the guard has to
// confirm the pid really belongs to a proxy.
func TestStaleP1dFileDoesNotBlockStartup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OPENCODEX_HOME", home)

	pidPath, portPath, err := runtimePaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o700); err != nil {
		t.Fatal(err)
	}
	// This test process is definitely alive and is definitely not a proxy --
	// exactly the shape of a recycled pid. Using a live pid matters: a dead one
	// never reaches the guard, so the test would pass without proving anything.
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	// Point the recorded port somewhere nothing is listening so the identity
	// probe fails fast rather than reaching a real proxy.
	if err := os.WriteFile(portPath, []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := writeRuntimeFiles(10199); err != nil {
		t.Fatalf("a stale pid file blocked startup: %v", err)
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := strconv.Atoi(string(data)); got != os.Getpid() {
		t.Fatalf("pid file = %s, want this process %d", data, os.Getpid())
	}
}
