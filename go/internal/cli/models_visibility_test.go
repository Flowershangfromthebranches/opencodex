package cli

import (
	"testing"
)

// A bare id means a native OpenAI slug. Without that fallback a user would have
// to type "openai/gpt-5.5" for a model every other command prints as "gpt-5.5".
func TestBareSelectorIsTreatedAsNative(t *testing.T) {
	provider, id, native, err := parseModelSelector("gpt-5.5", false)
	if err != nil {
		t.Fatal(err)
	}
	if provider != "openai" || id != "gpt-5.5" || !native {
		t.Fatalf("got %q/%q native=%v, want openai/gpt-5.5 native=true", provider, id, native)
	}
}

func TestProviderSelectorSplitsOnTheFirstSlash(t *testing.T) {
	provider, id, native, err := parseModelSelector("p/my-model", false)
	if err != nil {
		t.Fatal(err)
	}
	if provider != "p" || id != "my-model" || native {
		t.Fatalf("got %q/%q native=%v, want p/my-model native=false", provider, id, native)
	}
}

// --native forces the native reading even for a slug that contains a slash,
// which is how a native id containing a slash stays addressable.
func TestNativeFlagOverridesTheSlashSplit(t *testing.T) {
	provider, id, native, err := parseModelSelector("weird/native-id", true)
	if err != nil {
		t.Fatal(err)
	}
	if provider != "openai" || id != "weird/native-id" || !native {
		t.Fatalf("got %q/%q native=%v, want the whole selector kept as a native id", provider, id, native)
	}
}

func TestMalformedSelectorsAreRejected(t *testing.T) {
	for _, selector := range []string{"", "   ", "/model", "provider/"} {
		if _, _, _, err := parseModelSelector(selector, false); err == nil {
			t.Fatalf("accepted malformed selector %q", selector)
		}
	}
}
