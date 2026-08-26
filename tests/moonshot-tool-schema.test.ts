import { describe, expect, test } from "bun:test";
import { createOpenAIChatAdapter as createProductionAdapter } from "../src/adapters/openai-chat";
import { normalizeMoonshotToolParameters } from "../src/adapters/moonshot-tool-schema";
import type { OcxParsedRequest, OcxProviderConfig, OcxTool } from "../src/types";
import { withTestTranslatorBudget } from "./helpers/translator-budget";

const createAdapter = (provider: OcxProviderConfig) =>
  withTestTranslatorBudget(createProductionAdapter(provider));

const siblingRefSchema = {
  $schema: "https://json-schema.org/draft/2020-12/schema",
  type: "object",
  properties: { threadId: { $ref: "#/$defs/__schema20" } },
  required: ["threadId"],
  additionalProperties: false,
  $defs: {
    __schema2: { type: "string", minLength: 1 },
    __schema20: {
      type: "string",
      minLength: 1,
      format: "uuid",
      description: "Target thread UUID.",
      $ref: "#/$defs/__schema2",
    },
  },
};

function parsed(tool: OcxTool): OcxParsedRequest {
  return {
    modelId: "k3",
    context: {
      messages: [{ role: "user", content: "Reply OK", timestamp: 0 }],
      tools: [tool],
    },
    stream: false,
    options: {},
  };
}

function emittedParameters(baseUrl: string): Record<string, unknown> {
  const request = createAdapter({
    adapter: "openai-chat",
    baseUrl,
    apiKey: "test-key",
    authMode: "key",
  }).buildRequest(parsed({ name: "schema_probe", parameters: structuredClone(siblingRefSchema) }));
  const body = JSON.parse(request.body) as {
    tools: Array<{ function: { parameters: Record<string, unknown> } }>;
  };
  return body.tools[0]!.function.parameters;
}

describe("Moonshot tool schema compatibility", () => {
  test("inlines local refs that carry sibling validation keywords", () => {
    const normalized = normalizeMoonshotToolParameters(structuredClone(siblingRefSchema));
    expect(normalized?.$defs).toEqual({
      __schema2: { type: "string", minLength: 1 },
      __schema20: {
        type: "string",
        minLength: 1,
        format: "uuid",
        description: "Target thread UUID.",
      },
    });
    expect((normalized?.properties as Record<string, unknown>).threadId).toEqual({
      $ref: "#/$defs/__schema20",
    });
  });

  test("degrades an unresolvable sibling ref to a pure ref", () => {
    expect(normalizeMoonshotToolParameters({
      type: "object",
      properties: { value: { $ref: "#/$defs/missing", type: "string", minLength: 1 } },
    })?.properties).toEqual({ value: { $ref: "#/$defs/missing" } });
  });

  test("does not overwrite conflicting referenced assertions", () => {
    const normalized = normalizeMoonshotToolParameters({
      type: "object",
      properties: {
        value: {
          $ref: "#/$defs/Base",
          required: ["b"],
          properties: { b: { type: "number" } },
        },
      },
      $defs: {
        Base: {
          type: "object",
          required: ["a"],
          properties: { a: { type: "string" } },
        },
      },
    });

    expect((normalized?.properties as Record<string, unknown>).value).toEqual({
      $ref: "#/$defs/Base",
    });
  });

  test("does not replace a referenced enum or minimum with a sibling assertion", () => {
    const normalized = normalizeMoonshotToolParameters({
      type: "object",
      properties: {
        mode: { $ref: "#/$defs/Mode", enum: ["b"] },
        count: { $ref: "#/$defs/Count", minimum: 1 },
      },
      $defs: {
        Mode: { type: "string", enum: ["a"] },
        Count: { type: "integer", minimum: 5 },
      },
    });

    expect(normalized?.properties).toEqual({
      mode: { $ref: "#/$defs/Mode" },
      count: { $ref: "#/$defs/Count" },
    });
  });

  test.each([
    "https://api.kimi.com/coding/v1",
    "https://api.moonshot.ai/v1",
    "https://api.moonshot.cn/v1",
  ])("normalizes the first-party destination %s", baseUrl => {
    const parameters = emittedParameters(baseUrl);
    expect(JSON.stringify(parameters)).not.toContain('"$ref":"#/$defs/__schema2"');
    expect(JSON.stringify(parameters)).toContain('"format":"uuid"');
  });

  test("preserves a third-party OpenAI-chat gateway byte-shape", () => {
    expect(emittedParameters("https://gateway.example/v1")).toEqual(siblingRefSchema);
  });
});
