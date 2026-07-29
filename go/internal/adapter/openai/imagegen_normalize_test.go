package openai

import (
	"encoding/json"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/config"
	"github.com/lidge-jun/opencodex-go/internal/types"
)

// Cases ported from the oracle's own contract tests
// (tests/openai-responses-passthrough.test.ts:631-908), which are the most
// precise statement of what this normalization owes the client.

func normalizedBody(t *testing.T, raw string) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatal(err)
	}
	// Driven through the real request-sanitization entry point rather than
	// calling the normalizer directly: a test that bypasses the wiring passes
	// just as happily when the normalizer is never reached.
	sanitizeResponsesBodyForRequest(body, false, &types.NormalizedRequest{}, config.ProviderConfig{})
	return body
}

func toolNames(t *testing.T, tools any) []string {
	t.Helper()
	var names []string
	for _, raw := range sliceValue(tools) {
		tool, _ := raw.(map[string]any)
		if name := stringValue(tool["name"]); name != "" {
			names = append(names, name)
		} else {
			names = append(names, stringValue(tool["type"]))
		}
	}
	return names
}

// A dotted declaration becomes the upstream-safe alias, the hosted duplicate is
// dropped because something now replaces it, and unrelated hosted tools survive.
func TestImageGenReplacesDottedFunctionWithSafeAlias(t *testing.T) {
	body := normalizedBody(t, `{"model":"gpt-5.6-sol","input":[],"tools":[
		{"type":"function","name":"image_gen.imagegen","parameters":{}},
		{"type":"image_generation"},
		{"type":"web_search"}
	]}`)

	names := toolNames(t, body["tools"])
	if len(names) != 2 {
		t.Fatalf("tools = %v, want the hosted duplicate dropped", names)
	}
	if names[0] != "image_gen__imagegen" || names[1] != "web_search" {
		t.Fatalf("tools = %v", names)
	}
}

// A complete namespace is lowered to flat aliases, and every other field of the
// inner function declaration is carried across unchanged.
func TestImageGenFlattensNamespaceAndKeepsMetadata(t *testing.T) {
	body := normalizedBody(t, `{"model":"gpt-5.6-sol","input":[],"tools":[
		{"type":"namespace","name":"image_gen","description":"Client image tools","tools":[
			{"type":"function","name":"imagegen","description":"Generate or edit an image","parameters":{"type":"object"},"strict":true}
		]},
		{"type":"image_generation"}
	]}`)

	tools := sliceValue(body["tools"])
	if len(tools) != 1 {
		t.Fatalf("tools = %v", toolNames(t, body["tools"]))
	}
	tool, _ := tools[0].(map[string]any)
	if stringValue(tool["name"]) != "image_gen__imagegen" {
		t.Fatalf("name = %q", stringValue(tool["name"]))
	}
	if stringValue(tool["description"]) != "Generate or edit an image" || tool["strict"] != true {
		t.Fatalf("declaration metadata was lost: %#v", tool)
	}
}

// THE case the previous code got wrong: with no usable alias, the hosted tool is
// the client's only way to generate an image. Removing it unconditionally
// deleted the capability outright.
func TestImageGenKeepsHostedToolWhenNothingReplacesIt(t *testing.T) {
	body := normalizedBody(t, `{"model":"gpt-5.6-sol","input":[],"tools":[
		{"type":"function","name":"image_gen","parameters":{}},
		{"type":"image_generation"}
	]}`)

	names := toolNames(t, body["tools"])
	found := false
	for _, name := range names {
		if name == "image_generation" {
			found = true
		}
	}
	if !found {
		t.Fatalf("image generation was removed with nothing to replace it: %v", names)
	}
}

// A forced tool choice has to follow the rename, or it selects a tool that no
// longer exists in the request.
func TestImageGenRewritesToolChoice(t *testing.T) {
	body := normalizedBody(t, `{"model":"gpt-5.6-sol","input":[],
		"tools":[{"type":"function","name":"image_gen.imagegen","parameters":{}}],
		"tool_choice":{"type":"function","name":"image_gen.imagegen"}}`)

	choice, _ := body["tool_choice"].(map[string]any)
	if stringValue(choice["name"]) != "image_gen__imagegen" {
		t.Fatalf("tool_choice = %#v", choice)
	}
}

func TestImageGenRewritesAllowedToolsChoice(t *testing.T) {
	body := normalizedBody(t, `{"model":"gpt-5.6-sol","input":[],
		"tools":[{"type":"function","name":"image_gen.imagegen","parameters":{}}],
		"tool_choice":{"type":"allowed_tools","mode":"auto","tools":[
			{"type":"function","name":"image_gen.imagegen"},
			{"type":"function","name":"exec_command"}
		]}}`)

	choice, _ := body["tool_choice"].(map[string]any)
	names := toolNames(t, choice["tools"])
	if len(names) != 2 || names[0] != "image_gen__imagegen" || names[1] != "exec_command" {
		t.Fatalf("allowed tools = %v", names)
	}
}

// Responses Lite carries tools in a nested additional_tools item, and a
// namespace there is lowered without needing a hosted tool present.
func TestImageGenFlattensNestedAdditionalTools(t *testing.T) {
	body := normalizedBody(t, `{"model":"gpt-5.6-sol","tools":[],"input":[
		{"type":"additional_tools","role":"developer","tools":[
			{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen","parameters":{}}]},
			{"type":"web_search"}
		]},
		{"type":"message","role":"user","content":[]}
	]}`)

	input := sliceValue(body["input"])
	nested, _ := input[0].(map[string]any)
	if stringValue(nested["role"]) != "developer" {
		t.Fatalf("the additional_tools container lost its role: %#v", nested)
	}
	names := toolNames(t, nested["tools"])
	if len(names) != 2 || names[0] != "image_gen__imagegen" || names[1] != "web_search" {
		t.Fatalf("nested tools = %v", names)
	}
	if stringValue(input[1].(map[string]any)["type"]) != "message" {
		t.Fatalf("unrelated input items were disturbed: %#v", input[1])
	}
}

// A replayed call must name the alias its declaration received, or the follow-up
// turn references a tool the upstream never saw declared.
func TestImageGenEncodesReplayedCalls(t *testing.T) {
	body := normalizedBody(t, `{"model":"gpt-5.6-sol",
		"tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen","parameters":{}}]}],
		"input":[
			{"type":"function_call","namespace":"image_gen","name":"imagegen","call_id":"call_native","arguments":"{}"},
			{"type":"function_call","name":"image_gen.imagegen","call_id":"call_legacy","arguments":"{}"},
			{"type":"function_call","name":"exec_command","call_id":"call_other","arguments":"{}"}
		]}`)

	input := sliceValue(body["input"])
	native, _ := input[0].(map[string]any)
	if stringValue(native["name"]) != "image_gen__imagegen" {
		t.Fatalf("native call = %#v", native)
	}
	if _, present := native["namespace"]; present {
		t.Fatalf("the private namespace field was sent upstream: %#v", native)
	}
	if legacy, _ := input[1].(map[string]any); stringValue(legacy["name"]) != "image_gen__imagegen" {
		t.Fatalf("legacy call = %#v", input[1])
	}
	if other, _ := input[2].(map[string]any); stringValue(other["name"]) != "exec_command" {
		t.Fatalf("an unrelated call was rewritten: %#v", input[2])
	}
}

// Normalization runs on every request, so a body that already went through it
// must come out unchanged, and duplicate aliases must collapse to one.
func TestImageGenNormalizationIsIdempotentAndDeduplicates(t *testing.T) {
	raw := `{"model":"gpt-5.6-sol",
		"tools":[
			{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen","parameters":{"type":"object"}}]},
			{"type":"function","name":"image_gen.imagegen","parameters":{"type":"object"}}
		],
		"input":[{"type":"additional_tools","tools":[{"type":"web_search"}]}]}`

	first := normalizedBody(t, raw)
	names := toolNames(t, first["tools"])
	if len(names) != 1 || names[0] != "image_gen__imagegen" {
		t.Fatalf("duplicate aliases were not collapsed: %v", names)
	}

	encoded, _ := json.Marshal(first)
	second := normalizedBody(t, string(encoded))
	reencoded, _ := json.Marshal(second)
	if string(encoded) != string(reencoded) {
		t.Fatalf("normalization is not idempotent:\n%s\n%s", encoded, reencoded)
	}
}

// A request that never mentions image generation is left completely alone.
func TestImageGenLeavesUnrelatedRequestsUntouched(t *testing.T) {
	raw := `{"model":"gpt-5.6-sol","tools":[{"type":"function","name":"exec_command","parameters":{}},{"type":"image_generation"}],"input":[]}`
	body := normalizedBody(t, raw)
	names := toolNames(t, body["tools"])
	if len(names) != 2 || names[1] != "image_generation" {
		t.Fatalf("an unrelated request was rewritten: %v", names)
	}
}
