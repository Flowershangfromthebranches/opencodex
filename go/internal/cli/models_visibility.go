package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const modelsVisibilityUsage = `Usage:
  ocx models enable <provider/model|native-id> [--native] [--json]
  ocx models disable <provider/model|native-id> [--native] [--json]
  ocx models provider <provider> <on|off> [--json]`

// modelVisibilityTarget is one model the visibility route should act on.
type modelVisibilityTarget struct {
	ID     string `json:"id"`
	Native bool   `json:"native"`
}

// parseModelSelector splits "provider/model" and treats a bare id as a native
// OpenAI slug, matching the oracle (src/cli/models-runtime.ts:77). Without the
// native fallback a user would have to spell "openai/gpt-5.5" for a model the
// rest of the CLI prints as "gpt-5.5".
func parseModelSelector(selector string, forceNative bool) (provider, id string, native bool, err error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", "", false, fmt.Errorf("model selector is required\n%s", modelsVisibilityUsage)
	}
	if forceNative || !strings.Contains(selector, "/") {
		return "openai", selector, true, nil
	}
	slash := strings.Index(selector, "/")
	provider, id = selector[:slash], selector[slash+1:]
	if provider == "" || id == "" {
		return "", "", false, fmt.Errorf("model selector must be provider/model or a native model id\n%s", modelsVisibilityUsage)
	}
	return provider, id, false, nil
}

func modelsSetVisibility(enabled bool, args []string, streams IO) error {
	jsonOutput, rest := consumeBoolFlag(args, "--json")
	native, rest := consumeBoolFlag(rest, "--native")
	if len(rest) != 1 {
		return fmt.Errorf("model selector is required\n%s", modelsVisibilityUsage)
	}
	provider, id, isNative, err := parseModelSelector(rest[0], native)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"scope": "models", "provider": provider, "enabled": enabled,
		"targets": []modelVisibilityTarget{{ID: id, Native: isNative}},
	}
	body, err := managementRequest(http.MethodPut, "/api/model-visibility", payload)
	if err != nil {
		return err
	}
	if jsonOutput {
		_, err := streams.Out.Write(append(bytes.TrimSpace(body), '\n'))
		return err
	}
	verb := "Disabled"
	if enabled {
		verb = "Enabled"
	}
	fmt.Fprintf(streams.Out, "%s %s.\n", verb, rest[0])
	return nil
}

// modelsSetProviderVisibility flips every model a provider currently exposes.
// The target list comes from /api/models rather than being assumed, so the call
// only ever names models that actually exist right now.
func modelsSetProviderVisibility(args []string, streams IO) error {
	jsonOutput, rest := consumeBoolFlag(args, "--json")
	if len(rest) != 2 {
		return fmt.Errorf("provider and on|off are required\n%s", modelsVisibilityUsage)
	}
	provider := strings.TrimSpace(rest[0])
	state := strings.ToLower(strings.TrimSpace(rest[1]))
	if provider == "" || (state != "on" && state != "off") {
		return fmt.Errorf("provider and on|off are required\n%s", modelsVisibilityUsage)
	}

	listing, err := managementRequest(http.MethodGet, "/api/models", nil)
	if err != nil {
		return err
	}
	var rows []struct {
		Provider string `json:"provider"`
		ID       string `json:"id"`
		Native   bool   `json:"native"`
	}
	if err := json.Unmarshal(listing, &rows); err != nil {
		return fmt.Errorf("decode model list: %w", err)
	}
	targets := make([]modelVisibilityTarget, 0, len(rows))
	for _, row := range rows {
		if row.Provider == provider && strings.TrimSpace(row.ID) != "" {
			targets = append(targets, modelVisibilityTarget{ID: row.ID, Native: row.Native})
		}
	}
	if len(targets) == 0 {
		return fmt.Errorf("no models are available for provider %s", provider)
	}

	payload := map[string]any{"scope": "provider", "provider": provider, "enabled": state == "on", "targets": targets}
	body, err := managementRequest(http.MethodPut, "/api/model-visibility", payload)
	if err != nil {
		return err
	}
	if jsonOutput {
		_, err := streams.Out.Write(append(bytes.TrimSpace(body), '\n'))
		return err
	}
	fmt.Fprintf(streams.Out, "%s: %s\n", provider, state)
	return nil
}

// managementRequest talks to the running proxy's management API. These commands
// change LIVE runtime state, so they deliberately fail when no proxy is up
// rather than editing config behind a running process's back.
func managementRequest(method, path string, payload any) ([]byte, error) {
	cfg, _, err := loadConfig()
	if err != nil {
		return nil, err
	}
	_, port := readRuntime()
	if port <= 0 {
		return nil, fmt.Errorf("proxy is not running; start it before changing model visibility")
	}
	var reader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, serviceBaseURLAt(*cfg, port)+path, reader)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token := strings.TrimSpace(os.Getenv("OPENCODEX_API_AUTH_TOKEN")); token != "" {
		request.Header.Set("X-OpenCodex-API-Key", token)
	} else if cfg.AuthToken != "" {
		request.Header.Set("X-OpenCodex-API-Key", cfg.AuthToken)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", path, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s failed (%d): %s", method, path, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
