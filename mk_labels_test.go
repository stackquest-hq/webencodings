package webencodings

import (
	"testing"
)

func TestGenerateLabels(t *testing.T) {
	webencodingsURL := "http://encoding.spec.whatwg.org/encodings.json"
	result := GenerateLabels(webencodingsURL)

	if result == "" {
		t.Error("GenerateLabels returned empty string")
	}

	// Check if the result contains expected content
	if !contains(result, "package webencodings") {
		t.Error("Generated code should contain package declaration")
	}

	if !contains(result, "var Labels = map[string]string{") {
		t.Error("Generated code should contain Labels map declaration")
	}

	if !contains(result, "func GetCanonicalName") {
		t.Error("Generated code should contain GetCanonicalName function")
	}

	t.Log("Labels generation completed successfully")
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > len(substr) && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
