package management

import (
	"encoding/json"
	"net/http"
	"testing"
)

// These assert JSON KEY NAMES rather than Go struct fields on purpose. The key
// names are the contract the dashboard reads; a rename that keeps the Go type
// compiling would still break the page.

func TestCleanupPolicyGetCarriesJobState(t *testing.T) {
	api, _ := storageAPI(t, t.TempDir())
	response := serveManagement(api, http.MethodGet, "/api/storage/cleanup-policy", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	// The policy fields stay at the top level; job hangs beside them.
	for _, key := range []string{"enabled", "trigger", "target", "schedule", "mode", "job"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("GET response is missing %q: %s", key, response.Body.String())
		}
	}
	job, ok := body["job"].(map[string]any)
	if !ok {
		t.Fatalf("job is not an object: %v", body["job"])
	}
	// Nothing runs yet, so idle is the honest answer. This asserts the CURRENT
	// state and the shape -- not that the state is permanently idle, which would
	// turn wiring the executor into a false regression.
	if job["status"] != "idle" {
		t.Fatalf("job status = %v, want idle", job["status"])
	}
	for _, absent := range []string{"reason", "startedAt", "finishedAt", "lastError", "lastOutcome"} {
		if _, present := job[absent]; present {
			t.Fatalf("idle job should omit %q, got %v", absent, job)
		}
	}
}

func TestCleanupPolicyPutReturnsSaveEnvelope(t *testing.T) {
	api, _ := storageAPI(t, t.TempDir())
	response := serveManagement(api, http.MethodPut, "/api/storage/cleanup-policy", `{"enabled":true}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["ok"] != true {
		t.Fatalf("ok = %v, want true", body["ok"])
	}
	// The dashboard reads json.policy and reports a failed save without it.
	policy, ok := body["policy"].(map[string]any)
	if !ok {
		t.Fatalf("PUT response has no policy object: %s", response.Body.String())
	}
	if policy["enabled"] != true {
		t.Fatalf("saved policy did not keep enabled: %v", policy)
	}
	if _, ok := body["job"]; !ok {
		t.Fatalf("PUT response is missing job: %s", response.Body.String())
	}
}

// Omitting `enabled` must leave it exactly as it was, in BOTH directions. Only
// checking the off case would miss a write path that silently disables a policy
// the user turned on.
func TestCleanupPolicyPutPreservesOmittedEnabled(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		start bool
	}{
		{"stays off", false},
		{"stays on", true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			api, _ := storageAPI(t, t.TempDir())
			seed := `{"enabled":false}`
			if testCase.start {
				seed = `{"enabled":true}`
			}
			if response := serveManagement(api, http.MethodPut, "/api/storage/cleanup-policy", seed); response.Code != http.StatusOK {
				t.Fatalf("seed status=%d body=%s", response.Code, response.Body.String())
			}

			// A later save that changes something unrelated and omits `enabled`.
			response := serveManagement(api, http.MethodPut, "/api/storage/cleanup-policy", `{"mode":"permanent"}`)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			policy, ok := body["policy"].(map[string]any)
			if !ok {
				t.Fatalf("no policy object: %s", response.Body.String())
			}
			if policy["enabled"] != testCase.start {
				t.Fatalf("omitted enabled changed from %t to %v", testCase.start, policy["enabled"])
			}
		})
	}
}
