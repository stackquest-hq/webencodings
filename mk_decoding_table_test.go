package webencodings

import (
	"strings"
	"testing"
)

func TestGenerateDecodingTable(t *testing.T) {
	result := GenerateDecodingTable()

	if result == "" {
		t.Error("GenerateDecodingTable returned empty string")
	}

	// Check if the result contains expected content
	if !strings.Contains(result, "package webencodings") {
		t.Error("Generated code should contain package declaration")
	}

	if !strings.Contains(result, "var DecodingTable = [256]rune{") {
		t.Error("Generated code should contain DecodingTable array declaration")
	}

	t.Log("Decoding table generation completed successfully")
}
