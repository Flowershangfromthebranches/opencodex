package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const providerRuntimeUsage = `Usage:
  ocx provider quota [--refresh] [--json]
  ocx provider presets [--json]
  ocx provider account-mode <pool|direct> [--json]
  ocx provider selected <name> [--set <id,id...>|--clear] [--json]`

func providerQuota(args []string, streams IO) error {
	jsonOutput, rest := consumeBoolFlag(args, "--json")
	refresh, rest := consumeBoolFlag(rest, "--refresh")
	if len(rest) != 0 {
		return fmt.Errorf("unexpected arguments: %s\n%s", strings.Join(rest, " "), providerRuntimeUsage)
	}
	path := "/api/provider-quotas"
	if refresh {
		// Opt-in because refreshing calls every provider's quota endpoint; the
		// default read serves the cached report.
		path += "?refresh=1"
	}
	body, err := managementRequest(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return writeRuntimeResult(streams, body, jsonOutput, strings.TrimSpace(string(body)))
}

func providerPresets(args []string, streams IO) error {
	jsonOutput, rest := consumeBoolFlag(args, "--json")
	if len(rest) != 0 {
		return fmt.Errorf("unexpected arguments: %s\n%s", strings.Join(rest, " "), providerRuntimeUsage)
	}
	body, err := managementRequest(http.MethodGet, "/api/provider-presets", nil)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeRuntimeResult(streams, body, true)
	}
	// The route may answer with a bare array or an object wrapping one, so both
	// shapes are read rather than assuming the current server's choice.
	var wrapped struct {
		Providers []map[string]any `json:"providers"`
	}
	rows := wrapped.Providers
	if err := json.Unmarshal(body, &wrapped); err != nil || wrapped.Providers == nil {
		var bare []map[string]any
		if err := json.Unmarshal(body, &bare); err != nil {
			return fmt.Errorf("decode provider presets: %w", err)
		}
		rows = bare
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		id, _ := row["id"].(string)
		if id == "" {
			id, _ = row["name"].(string)
		}
		label, _ := row["label"].(string)
		if label == "" {
			label, _ = row["adapter"].(string)
		}
		lines = append(lines, strings.TrimSpace(id+"  "+label))
	}
	return writeRuntimeResult(streams, nil, false, lines...)
}

func providerAccountMode(args []string, streams IO) error {
	jsonOutput, rest := consumeBoolFlag(args, "--json")
	if len(rest) != 1 {
		return fmt.Errorf("mode must be pool or direct\n%s", providerRuntimeUsage)
	}
	mode := strings.ToLower(strings.TrimSpace(rest[0]))
	if mode != "pool" && mode != "direct" {
		return fmt.Errorf("mode must be pool or direct\n%s", providerRuntimeUsage)
	}
	body, err := managementRequest(http.MethodPatch, "/api/providers?name=openai", map[string]any{"codexAccountMode": mode})
	if err != nil {
		return err
	}
	return writeRuntimeResult(streams, body, jsonOutput, "OpenAI Codex account mode: "+mode)
}
