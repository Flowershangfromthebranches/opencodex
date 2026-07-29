package cli

import (
	"fmt"

	"github.com/lidge-jun/opencodex-go/internal/codex"
)

// runDoctorFixCodexRuntime adopts a newer Codex runtime when probing finds one.
//
// Doctor already reports that a newer runtime exists; without this flag the
// user had to work out where it lived and edit config by hand. The three
// outcomes are distinct on purpose: nothing newer, an environment override that
// this cannot change, and an actual switch — collapsing them would leave
// someone with CODEX_CLI_PATH set wondering why the "fix" did nothing.
func runDoctorFixCodexRuntime(streams IO) error {
	home, err := configDir()
	if err != nil {
		return err
	}
	options := codex.ResolveCodexRuntimeOptions{ConfigDir: home}
	resolved := codex.ResolveCodexRuntime(options)

	if resolved.NewerAvailable == nil {
		fmt.Fprintln(streams.Out, "No newer Codex runtime found; keeping current selection.")
		current := codex.ResolveAndPersistCodexRuntime(options)
		fmt.Fprintf(streams.Out, "Selected: %s (%s)\n",
			codex.DisplayCodexRuntimePath(current.Runtime.Command, home),
			runtimeVersionOrUnknown(current.Runtime.Version))
		return nil
	}
	if resolved.Runtime.Source == codex.RuntimeSourceEnvironment {
		// The env var wins over anything written to config, so persisting here
		// would claim a change the next run ignores.
		fmt.Fprintln(streams.Out, "CODEX_CLI_PATH currently overrides configured runtimes.")
		fmt.Fprintf(streams.Out, "Unset or update CODEX_CLI_PATH to use %s (%s).\n",
			codex.DisplayCodexRuntimePath(resolved.NewerAvailable.Command, home),
			runtimeVersionOrUnknown(resolved.NewerAvailable.Version))
		fmt.Fprintln(streams.Out, "Then run ocx sync.")
		return nil
	}
	selected := *resolved.NewerAvailable
	selected.Source = codex.RuntimeSourceConfigured
	if err := codex.PersistCodexRuntime(selected, options); err != nil {
		return fmt.Errorf("persist Codex runtime: %w", err)
	}
	fmt.Fprintf(streams.Out, "Updated Codex runtime to %s (%s).\n",
		codex.DisplayCodexRuntimePath(selected.Command, home),
		runtimeVersionOrUnknown(selected.Version))
	fmt.Fprintln(streams.Out, "Run ocx sync to refresh the catalog against this runtime.")
	return nil
}

func runtimeVersionOrUnknown(version string) string {
	if version == "" {
		return "unknown"
	}
	return version
}
