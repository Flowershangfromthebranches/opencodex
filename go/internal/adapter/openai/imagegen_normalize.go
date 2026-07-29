package openai

import "strings"

// Codex declares image generation under a private `image_gen` namespace, but the
// public Responses API reserves that namespace and restricts function names to a
// flat alphabet. `image_gen__<tool>` is therefore an upstream-only alias; the
// server restores the namespaced form on the way back.
const (
	imageGenNamespace     = "image_gen"
	imageGenDottedPrefix  = imageGenNamespace + "."
	imageGenWirePrefix    = imageGenNamespace + "__"
	hostedImageGeneration = "image_generation"
)

func imageGenLocalName(name string) string {
	if rest, found := strings.CutPrefix(name, imageGenDottedPrefix); found {
		return rest
	}
	if rest, found := strings.CutPrefix(name, imageGenWirePrefix); found {
		return rest
	}
	return name
}

func imageGenWireName(name string) string { return imageGenWirePrefix + imageGenLocalName(name) }

func isImageGenClientName(name string) bool {
	return name == imageGenNamespace ||
		strings.HasPrefix(name, imageGenDottedPrefix) ||
		strings.HasPrefix(name, imageGenWirePrefix)
}

func declaresImageGenClientTool(value any) bool {
	tool, ok := value.(map[string]any)
	if !ok {
		return false
	}
	name := stringValue(tool["name"])
	if name == "" {
		return false
	}
	if stringValue(tool["type"]) == "namespace" {
		return name == imageGenNamespace
	}
	return isImageGenClientName(name)
}

// flattenImageGenNamespace lowers one complete namespace to public function
// tools. Only a non-empty namespace of named function tools is safe to lower:
// malformed, empty, and future shapes are left untouched rather than silently
// losing the client's capability.
func flattenImageGenNamespace(value any) []map[string]any {
	tool, ok := value.(map[string]any)
	if !ok || stringValue(tool["type"]) != "namespace" || stringValue(tool["name"]) != imageGenNamespace {
		return nil
	}
	inner := sliceValue(tool["tools"])
	if len(inner) == 0 {
		return nil
	}
	flattened := make([]map[string]any, 0, len(inner))
	for _, raw := range inner {
		entry, ok := raw.(map[string]any)
		if !ok || stringValue(entry["type"]) != "function" || stringValue(entry["name"]) == "" {
			return nil
		}
		lowered := cloneStringKeyedMap(entry)
		lowered["name"] = imageGenWireName(stringValue(entry["name"]))
		flattened = append(flattened, lowered)
	}
	return flattened
}

func normalizeFlatImageGenFunction(value any) (any, bool) {
	tool, ok := value.(map[string]any)
	if !ok || stringValue(tool["type"]) != "function" {
		return value, false
	}
	name := stringValue(tool["name"])
	if !strings.HasPrefix(name, imageGenDottedPrefix) {
		return value, false
	}
	normalized := cloneStringKeyedMap(tool)
	normalized["name"] = imageGenWireName(name)
	return normalized, true
}

func imageGenFunctionName(value any) string {
	tool, ok := value.(map[string]any)
	if !ok || stringValue(tool["type"]) != "function" {
		return ""
	}
	if name := stringValue(tool["name"]); isImageGenClientName(name) {
		return name
	}
	return ""
}

// declaresUsableImageGenAlias reports whether a declaration can yield a callable
// upstream-safe alias. The hosted image_generation tool is removed ONLY when one
// exists — dropping it unconditionally, as the previous code did, deleted the
// client's image-generation capability whenever nothing could replace it.
func declaresUsableImageGenAlias(value any) bool {
	if flattenImageGenNamespace(value) != nil {
		return true
	}
	tool, ok := value.(map[string]any)
	if !ok || stringValue(tool["type"]) != "function" {
		return false
	}
	name := stringValue(tool["name"])
	if strings.HasPrefix(name, imageGenDottedPrefix) {
		return len(name) > len(imageGenDottedPrefix)
	}
	return strings.HasPrefix(name, imageGenWirePrefix) && len(name) > len(imageGenWirePrefix)
}

func collectResponsesToolGroups(body map[string]any) [][]any {
	var groups [][]any
	if tools := sliceValue(body["tools"]); tools != nil {
		groups = append(groups, tools)
	}
	for _, raw := range sliceValue(body["input"]) {
		item, ok := raw.(map[string]any)
		if !ok || stringValue(item["type"]) != "additional_tools" {
			continue
		}
		if tools := sliceValue(item["tools"]); tools != nil {
			groups = append(groups, tools)
		}
	}
	return groups
}

// imageGenToolChoiceAliases maps each selector name the client might use to the
// alias its declaration actually received, so a tool_choice keeps pointing at a
// tool that still exists after normalization.
func imageGenToolChoiceAliases(groups [][]any) map[string]string {
	aliases := make(map[string]string)
	for _, group := range groups {
		for _, value := range group {
			if flattened := flattenImageGenNamespace(value); flattened != nil {
				for _, candidate := range flattened {
					wire := stringValue(candidate["name"])
					aliases[imageGenDottedPrefix+imageGenLocalName(wire)] = wire
					aliases[wire] = wire
				}
				continue
			}
			tool, ok := value.(map[string]any)
			if !ok || stringValue(tool["type"]) != "function" {
				continue
			}
			name := stringValue(tool["name"])
			switch {
			case strings.HasPrefix(name, imageGenDottedPrefix) && len(name) > len(imageGenDottedPrefix):
				aliases[name] = imageGenWireName(name)
			case strings.HasPrefix(name, imageGenWirePrefix) && len(name) > len(imageGenWirePrefix):
				aliases[name] = name
			}
		}
	}
	return aliases
}

func normalizeImageGenToolChoice(value any, aliases map[string]string) (any, bool) {
	choice, ok := value.(map[string]any)
	if !ok {
		return value, false
	}
	switch stringValue(choice["type"]) {
	case "function":
		alias, found := aliases[stringValue(choice["name"])]
		if !found || alias == stringValue(choice["name"]) {
			return value, false
		}
		normalized := cloneStringKeyedMap(choice)
		normalized["name"] = alias
		return normalized, true
	case "allowed_tools":
		tools := sliceValue(choice["tools"])
		if tools == nil {
			return value, false
		}
		changed := false
		next := make([]any, 0, len(tools))
		for _, raw := range tools {
			tool, ok := raw.(map[string]any)
			if !ok || stringValue(tool["type"]) != "function" {
				next = append(next, raw)
				continue
			}
			alias, found := aliases[stringValue(tool["name"])]
			if !found || alias == stringValue(tool["name"]) {
				next = append(next, raw)
				continue
			}
			rewritten := cloneStringKeyedMap(tool)
			rewritten["name"] = alias
			next = append(next, rewritten)
			changed = true
		}
		if !changed {
			return value, false
		}
		normalized := cloneStringKeyedMap(choice)
		normalized["tools"] = next
		return normalized, true
	}
	return value, false
}

func declaresImageGenFunctionCall(value any) bool {
	item, ok := value.(map[string]any)
	if !ok || stringValue(item["type"]) != "function_call" {
		return false
	}
	name := stringValue(item["name"])
	if name == "" {
		return false
	}
	return stringValue(item["namespace"]) == imageGenNamespace || isImageGenClientName(name)
}

// normalizeImageGenFunctionCall encodes a REPLAYED call to the same alias its
// declaration received. Without this a follow-up turn replays a call naming a
// tool the upstream never saw declared.
func normalizeImageGenFunctionCall(value any) (any, bool) {
	if !declaresImageGenFunctionCall(value) {
		return value, false
	}
	item := value.(map[string]any)
	name := stringValue(item["name"])
	if stringValue(item["namespace"]) == imageGenNamespace {
		normalized := cloneStringKeyedMap(item)
		delete(normalized, "namespace")
		normalized["name"] = imageGenWireName(name)
		return normalized, true
	}
	if strings.HasPrefix(name, imageGenDottedPrefix) {
		normalized := cloneStringKeyedMap(item)
		normalized["name"] = imageGenWireName(name)
		return normalized, true
	}
	return value, false
}

// normalizeImageGenClientTools ports the oracle's normalization
// (src/adapters/openai-responses.ts:701-782) for API-key Responses providers.
func normalizeImageGenClientTools(body map[string]any) {
	groups := collectResponsesToolGroups(body)
	declared := false
	for _, group := range groups {
		for _, tool := range group {
			if declaresImageGenClientTool(tool) {
				declared = true
				break
			}
		}
	}
	if !declared {
		for _, item := range sliceValue(body["input"]) {
			if declaresImageGenFunctionCall(item) {
				declared = true
				break
			}
		}
	}
	if !declared {
		return
	}

	usableAlias := false
	for _, group := range groups {
		for _, tool := range group {
			if declaresUsableImageGenAlias(tool) {
				usableAlias = true
				break
			}
		}
	}
	aliases := imageGenToolChoiceAliases(groups)
	seen := make(map[string]bool)

	normalizeGroup := func(tools []any) ([]any, bool) {
		normalized := make([]any, 0, len(tools))
		changed := false
		for _, tool := range tools {
			if entry, ok := tool.(map[string]any); ok && usableAlias && stringValue(entry["type"]) == hostedImageGeneration {
				changed = true
				continue
			}
			candidates := []any{tool}
			if flattened := flattenImageGenNamespace(tool); flattened != nil {
				changed = true
				candidates = candidates[:0]
				for _, entry := range flattened {
					candidates = append(candidates, entry)
				}
			}
			for _, candidate := range candidates {
				normalizedCandidate, rewritten := normalizeFlatImageGenFunction(candidate)
				if rewritten {
					changed = true
				}
				if name := imageGenFunctionName(normalizedCandidate); name != "" {
					if seen[name] {
						changed = true
						continue
					}
					seen[name] = true
				}
				normalized = append(normalized, normalizedCandidate)
			}
		}
		if !changed {
			return tools, false
		}
		return normalized, true
	}

	if tools := sliceValue(body["tools"]); tools != nil {
		if next, changed := normalizeGroup(tools); changed {
			body["tools"] = next
		}
	}
	if input := sliceValue(body["input"]); input != nil {
		next := make([]any, 0, len(input))
		changed := false
		for _, raw := range input {
			if item, ok := raw.(map[string]any); ok && stringValue(item["type"]) == "additional_tools" {
				if nested := sliceValue(item["tools"]); nested != nil {
					if normalizedNested, nestedChanged := normalizeGroup(nested); nestedChanged {
						rewritten := cloneStringKeyedMap(item)
						rewritten["tools"] = normalizedNested
						next = append(next, rewritten)
						changed = true
						continue
					}
				}
				next = append(next, raw)
				continue
			}
			normalizedCall, callChanged := normalizeImageGenFunctionCall(raw)
			if callChanged {
				changed = true
			}
			next = append(next, normalizedCall)
		}
		if changed {
			body["input"] = next
		}
	}
	if choice, ok := body["tool_choice"]; ok {
		if normalized, changed := normalizeImageGenToolChoice(choice, aliases); changed {
			body["tool_choice"] = normalized
		}
	}
}

func cloneStringKeyedMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
