package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const modelsRuntimeUsage = `Usage:
  ocx models live [--provider <name>] [--json]
  ocx models edit <custom-id> [--model-id <id>] [--display-name <name|->]
      [--context-window <tokens|0>] [--modalities <text,image,audio|->] [--json]
  ocx models selected <provider> [--set <id,id...>|--clear] [--json]
  ocx models context <status|value <tokens>|provider <name> <on|off>|all <on|off>> [--json]
  ocx models shadow <status|set> [model|-] [--enabled <on|off>] [--json]`

// writeRuntimeResult prints the raw JSON when asked, otherwise the supplied
// human lines. Every verb below shares it so `--json` behaves identically
// across them rather than each one inventing its own formatting.
func writeRuntimeResult(streams IO, body []byte, jsonOutput bool, lines ...string) error {
	if jsonOutput {
		_, err := streams.Out.Write(append(bytes.TrimSpace(body), '\n'))
		return err
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(streams.Out, line); err != nil {
			return err
		}
	}
	return nil
}

// modelsLive lists what the RUNNING proxy currently exposes, which is not the
// same as the configured catalog: live-discovery providers gain models at
// runtime, and disabled ones stay listed but marked.
func modelsLive(args []string, streams IO) error {
	jsonOutput, rest := consumeBoolFlag(args, "--json")
	provider, rest, err := consumeValueFlag(rest, "--provider")
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return fmt.Errorf("unexpected arguments: %s\n%s", strings.Join(rest, " "), modelsRuntimeUsage)
	}
	body, err := managementRequest(http.MethodGet, "/api/models", nil)
	if err != nil {
		return err
	}
	var rows []struct {
		Provider   string `json:"provider"`
		ID         string `json:"id"`
		Namespaced string `json:"namespaced"`
		Native     bool   `json:"native"`
		Custom     bool   `json:"custom"`
		Disabled   bool   `json:"disabled"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return fmt.Errorf("decode model list: %w", err)
	}
	lines := make([]string, 0, len(rows))
	filtered := make([]any, 0, len(rows))
	for _, row := range rows {
		if provider != "" && row.Provider != provider {
			continue
		}
		flags := []string{"routed"}
		if row.Native {
			flags[0] = "native"
		}
		if row.Custom {
			flags = append(flags, "custom")
		}
		if row.Disabled {
			flags = append(flags, "disabled")
		} else {
			flags = append(flags, "enabled")
		}
		name := row.Namespaced
		if name == "" {
			name = row.Provider + "/" + row.ID
		}
		lines = append(lines, fmt.Sprintf("%s  [%s]", name, strings.Join(flags, ", ")))
		filtered = append(filtered, row)
	}
	if jsonOutput {
		// Re-encoded rather than echoed, because --provider filters the list and
		// echoing the raw body would ignore the filter the user asked for.
		encoded, err := json.MarshalIndent(filtered, "", "  ")
		if err != nil {
			return err
		}
		return writeRuntimeResult(streams, encoded, true)
	}
	return writeRuntimeResult(streams, nil, false, lines...)
}

func modelsEdit(args []string, streams IO) error {
	jsonOutput, rest := consumeBoolFlag(args, "--json")
	if len(rest) == 0 {
		return fmt.Errorf("custom model id is required\n%s", modelsRuntimeUsage)
	}
	id := strings.TrimSpace(rest[0])
	rest = rest[1:]
	if id == "" {
		return fmt.Errorf("custom model id is required\n%s", modelsRuntimeUsage)
	}
	patch := map[string]any{}
	modelID, rest, err := consumeValueFlag(rest, "--model-id")
	if err != nil {
		return err
	}
	if modelID != "" {
		patch["modelId"] = modelID
	}
	displayName, rest, err := consumeValueFlag(rest, "--display-name")
	if err != nil {
		return err
	}
	if displayName != "" {
		// "-" clears the field, matching the oracle: an empty string on the wire
		// is how the route distinguishes "clear" from "leave alone".
		if displayName == "-" {
			patch["displayName"] = ""
		} else {
			patch["displayName"] = displayName
		}
	}
	contextRaw, rest, err := consumeValueFlag(rest, "--context-window")
	if err != nil {
		return err
	}
	if contextRaw != "" {
		value, err := strconv.Atoi(strings.NewReplacer("_", "", ",", "").Replace(contextRaw))
		if err != nil || value < 0 {
			return fmt.Errorf("context window must be a non-negative integer\n%s", modelsRuntimeUsage)
		}
		patch["contextWindow"] = value
	}
	modalitiesRaw, rest, err := consumeValueFlag(rest, "--modalities")
	if err != nil {
		return err
	}
	if modalitiesRaw != "" {
		if modalitiesRaw == "-" {
			patch["inputModalities"] = []string{}
		} else {
			patch["inputModalities"] = splitCSV(modalitiesRaw)
		}
	}
	if len(rest) != 0 {
		return fmt.Errorf("unexpected arguments: %s\n%s", strings.Join(rest, " "), modelsRuntimeUsage)
	}
	if len(patch) == 0 {
		return fmt.Errorf("at least one field to change is required\n%s", modelsRuntimeUsage)
	}
	body, err := managementRequest(http.MethodPut, "/api/custom-models/"+id, patch)
	if err != nil {
		return err
	}
	return writeRuntimeResult(streams, body, jsonOutput, "Updated custom model "+id+".")
}

func modelsSelected(args []string, streams IO) error {
	jsonOutput, rest := consumeBoolFlag(args, "--json")
	clear, rest := consumeBoolFlag(rest, "--clear")
	setRaw, rest, err := consumeValueFlag(rest, "--set")
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("provider is required\n%s", modelsRuntimeUsage)
	}
	provider := strings.TrimSpace(rest[0])
	if provider == "" {
		return fmt.Errorf("provider is required\n%s", modelsRuntimeUsage)
	}
	if setRaw != "" && clear {
		return fmt.Errorf("--set and --clear cannot be combined\n%s", modelsRuntimeUsage)
	}
	if setRaw == "" && !clear {
		body, err := managementRequest(http.MethodGet, "/api/selected-models", nil)
		if err != nil {
			return err
		}
		var result struct {
			Selected  map[string][]string `json:"selected"`
			Available map[string][]string `json:"available"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("decode selected models: %w", err)
		}
		chosen := result.Selected[provider]
		summary := "all models"
		if len(chosen) > 0 {
			summary = strings.Join(chosen, ", ")
		}
		view, err := json.MarshalIndent(map[string]any{
			"provider": provider, "selected": chosen, "available": result.Available[provider],
		}, "", "  ")
		if err != nil {
			return err
		}
		return writeRuntimeResult(streams, view, jsonOutput, provider+": "+summary)
	}
	models := []string{}
	if !clear {
		models = splitCSV(setRaw)
	}
	body, err := managementRequest(http.MethodPut, "/api/selected-models", map[string]any{"provider": provider, "models": models})
	if err != nil {
		return err
	}
	summary := "all models"
	if len(models) > 0 {
		summary = strings.Join(models, ", ")
	}
	return writeRuntimeResult(streams, body, jsonOutput, provider+": "+summary)
}

func modelsContext(args []string, streams IO) error {
	jsonOutput, rest := consumeBoolFlag(args, "--json")
	action := "status"
	if len(rest) > 0 {
		action = strings.ToLower(strings.TrimSpace(rest[0]))
		rest = rest[1:]
	}
	if action == "status" {
		if len(rest) != 0 {
			return fmt.Errorf("unexpected arguments: %s\n%s", strings.Join(rest, " "), modelsRuntimeUsage)
		}
		body, err := managementRequest(http.MethodGet, "/api/provider-context-caps", nil)
		if err != nil {
			return err
		}
		return writeRuntimeResult(streams, body, jsonOutput, strings.TrimSpace(string(bytes.TrimSpace(body))))
	}
	payload := map[string]any{}
	switch action {
	case "value":
		if len(rest) < 1 {
			return fmt.Errorf("context value is required\n%s", modelsRuntimeUsage)
		}
		value, err := strconv.Atoi(strings.NewReplacer("_", "", ",", "").Replace(rest[0]))
		if err != nil || value <= 0 {
			return fmt.Errorf("context value must be a positive integer\n%s", modelsRuntimeUsage)
		}
		payload["value"], rest = value, rest[1:]
	case "provider":
		if len(rest) < 2 {
			return fmt.Errorf("provider and on|off are required\n%s", modelsRuntimeUsage)
		}
		state := strings.ToLower(strings.TrimSpace(rest[1]))
		if strings.TrimSpace(rest[0]) == "" || (state != "on" && state != "off") {
			return fmt.Errorf("provider and on|off are required\n%s", modelsRuntimeUsage)
		}
		payload["provider"], payload["enabled"], rest = strings.TrimSpace(rest[0]), state == "on", rest[2:]
	case "all":
		if len(rest) < 1 {
			return fmt.Errorf("all requires on|off\n%s", modelsRuntimeUsage)
		}
		state := strings.ToLower(strings.TrimSpace(rest[0]))
		if state != "on" && state != "off" {
			return fmt.Errorf("all requires on|off\n%s", modelsRuntimeUsage)
		}
		payload["setAll"], rest = state == "on", rest[1:]
	default:
		return fmt.Errorf("unknown context action %s\n%s", action, modelsRuntimeUsage)
	}
	if len(rest) != 0 {
		return fmt.Errorf("unexpected arguments: %s\n%s", strings.Join(rest, " "), modelsRuntimeUsage)
	}
	body, err := managementRequest(http.MethodPut, "/api/provider-context-caps", payload)
	if err != nil {
		return err
	}
	return writeRuntimeResult(streams, body, jsonOutput, "Context cap settings updated.")
}

func modelsShadow(args []string, streams IO) error {
	jsonOutput, rest := consumeBoolFlag(args, "--json")
	enabledRaw, rest, err := consumeValueFlag(rest, "--enabled")
	if err != nil {
		return err
	}
	action := "status"
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "--") {
		action = strings.ToLower(strings.TrimSpace(rest[0]))
		rest = rest[1:]
	}
	if action == "status" {
		if len(rest) != 0 || enabledRaw != "" {
			return fmt.Errorf("shadow status takes no options\n%s", modelsRuntimeUsage)
		}
		body, err := managementRequest(http.MethodGet, "/api/shadow-call-settings", nil)
		if err != nil {
			return err
		}
		return writeRuntimeResult(streams, body, jsonOutput, strings.TrimSpace(string(bytes.TrimSpace(body))))
	}
	if action != "set" {
		return fmt.Errorf("unknown shadow action %s\n%s", action, modelsRuntimeUsage)
	}
	payload := map[string]any{}
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "--") {
		if rest[0] == "-" {
			payload["model"] = ""
		} else {
			payload["model"] = strings.TrimSpace(rest[0])
		}
		rest = rest[1:]
	}
	if enabledRaw != "" {
		state := strings.ToLower(strings.TrimSpace(enabledRaw))
		if state != "on" && state != "off" {
			return fmt.Errorf("--enabled must be on or off\n%s", modelsRuntimeUsage)
		}
		payload["enabled"] = state == "on"
	}
	if len(rest) != 0 {
		return fmt.Errorf("unexpected arguments: %s\n%s", strings.Join(rest, " "), modelsRuntimeUsage)
	}
	if len(payload) == 0 {
		return fmt.Errorf("model and/or --enabled is required\n%s", modelsRuntimeUsage)
	}
	body, err := managementRequest(http.MethodPut, "/api/shadow-call-settings", payload)
	if err != nil {
		return err
	}
	return writeRuntimeResult(streams, body, jsonOutput, "Shadow-call settings updated.")
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
