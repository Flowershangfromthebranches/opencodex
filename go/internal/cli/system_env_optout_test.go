//go:build darwin

package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/config"
)

// Opting out must CLEAN UP, not just decline to install.
//
// Teardown otherwise only runs on a graceful stop, so a crash, a kill, or a
// reboot strands the user's shell and launchctl pointing at a proxy they
// switched off, with no owner left to undo it. That is a consent failure: the
// routing outlives the setting that authorized it.
func TestOptingOutRemovesAPreviousInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	opencodexDir := filepath.Join(home, ".opencodex")
	if err := os.MkdirAll(opencodexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(opencodexDir, "system-env.sh")
	trackingPath := filepath.Join(opencodexDir, "system-env.json")
	if err := os.WriteFile(envPath, []byte("export ANTHROPIC_BASE_URL='http://127.0.0.1:10100'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trackingPath, []byte(`{"values":{},"profile":""}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.ClaudeCode = &config.ClaudeCodeConfig{}

	installed, err := installSystemEnv(context.Background(), cfg, 10100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if installed {
		t.Fatal("installed without opt-in")
	}
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Fatalf("a stale system-env.sh survived opting out (%v)", err)
	}
	if _, err := os.Stat(trackingPath); !os.IsNotExist(err) {
		t.Fatalf("a stale system-env.json survived opting out (%v)", err)
	}
}
