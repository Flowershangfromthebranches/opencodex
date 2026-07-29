package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const providerEditUsage = `Usage: ocx provider edit <name> [options]

Options:
  --adapter <id>              Change the wire adapter
  --base-url <url>            Change the upstream base URL
  --default-model <id|->      Set or clear ("-") the default model
  --auth-mode <mode|->        Set or clear the auth mode (key, forward, oauth, local)
  --enabled <true|false>      Enable or disable the provider
  --live-models <true|false>  Toggle live model discovery
  --allow-private-network <b> Allow upstreams on private networks
  --json                      Print the raw response`

// providerEdit patches a live provider through the running proxy.
//
// The management route already served PATCH /api/providers; only the CLI verb
// was missing, so anyone scripting against the Go CLI had to fall back to curl
// or the dashboard for a change the oracle exposes as one command
// (oracle: src/cli/provider-runtime.ts:30).
func providerEdit(args []string, streams IO) error {
	jsonOutput, rest := consumeBoolFlag(args, "--json")
	if len(rest) == 0 {
		return fmt.Errorf("provider name is required\n%s", providerEditUsage)
	}
	name := strings.TrimSpace(rest[0])
	if name == "" {
		return fmt.Errorf("provider name is required\n%s", providerEditUsage)
	}
	patch, err := providerEditPatch(rest[1:])
	if err != nil {
		return err
	}
	if len(patch) == 0 {
		return fmt.Errorf("at least one edit option is required\n%s", providerEditUsage)
	}

	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	if _, ok := cfg.Providers[name]; !ok {
		return fmt.Errorf("provider %q is not configured", name)
	}
	_, port := readRuntime()
	if port <= 0 {
		return fmt.Errorf("proxy is not running; start it before editing a provider")
	}

	body, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	endpoint := serviceBaseURLAt(*cfg, port) + "/api/providers?" + url.Values{"name": {name}}.Encode()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(os.Getenv("OPENCODEX_API_AUTH_TOKEN")); token != "" {
		request.Header.Set("X-OpenCodex-API-Key", token)
	} else if cfg.AuthToken != "" {
		request.Header.Set("X-OpenCodex-API-Key", cfg.AuthToken)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("provider edit request: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read provider edit response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("provider edit failed (%d): %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	if jsonOutput {
		_, err := streams.Out.Write(append(bytes.TrimSpace(payload), '\n'))
		return err
	}
	fmt.Fprintf(streams.Out, "Updated provider %s.\n", name)
	return nil
}

// providerEditPatch mirrors the oracle's option set, including its "-" clearing
// convention: an explicit dash sends null so a field can be UNSET, which is
// different from omitting the flag and leaving it alone.
func providerEditPatch(args []string) (map[string]any, error) {
	patch := map[string]any{}
	rest := args
	for _, option := range []struct {
		flag  string
		key   string
		clear bool
	}{
		{flag: "--adapter", key: "adapter"},
		{flag: "--base-url", key: "baseUrl"},
		{flag: "--default-model", key: "defaultModel", clear: true},
		{flag: "--auth-mode", key: "authMode", clear: true},
	} {
		value, remaining, found, err := consumeStringOption(rest, option.flag)
		if err != nil {
			return nil, err
		}
		rest = remaining
		if !found {
			continue
		}
		if option.clear && value == "-" {
			// The route validates these as strings, so clearing sends an empty
			// string rather than null. Sending null would fail the type check
			// and the field would stay set.
			patch[option.key] = ""
			continue
		}
		patch[option.key] = value
	}
	for _, option := range []struct {
		flag   string
		key    string
		invert bool
	}{
		{flag: "--enabled", key: "disabled", invert: true},
		{flag: "--live-models", key: "liveModels"},
		{flag: "--allow-private-network", key: "allowPrivateNetwork"},
	} {
		value, remaining, found, err := consumeBoolOption(rest, option.flag)
		if err != nil {
			return nil, err
		}
		rest = remaining
		if !found {
			continue
		}
		if option.invert {
			patch[option.key] = !value
			continue
		}
		patch[option.key] = value
	}
	if len(rest) > 0 {
		return nil, fmt.Errorf("unexpected argument %q\n%s", rest[0], providerEditUsage)
	}
	return patch, nil
}

func consumeStringOption(args []string, flag string) (string, []string, bool, error) {
	for index, arg := range args {
		if arg != flag {
			continue
		}
		if index+1 >= len(args) {
			return "", nil, false, fmt.Errorf("%s requires a value", flag)
		}
		value := args[index+1]
		remaining := append(append([]string(nil), args[:index]...), args[index+2:]...)
		return value, remaining, true, nil
	}
	return "", args, false, nil
}

func consumeBoolOption(args []string, flag string) (bool, []string, bool, error) {
	value, remaining, found, err := consumeStringOption(args, flag)
	if err != nil || !found {
		return false, remaining, found, err
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "1":
		return true, remaining, true, nil
	case "false", "no", "0":
		return false, remaining, true, nil
	default:
		return false, nil, false, fmt.Errorf("%s expects true or false, got %q", flag, value)
	}
}
