package extractor

import (
	"encoding/json"
	"testing"
)

func TestResolveNodeConditional(t *testing.T) {
	jsonData := []byte(`{
		"isee": 15000.50,
		"nome": "Mario Rossi",
		"anni": [
			{
				"annualita1": "20232024",
				"corrispettivo": 318.0
			},
			{
				"annualita1": "20242025",
				"corrispettivo": 450.5
			}
		]
	}`)

	raw := json.RawMessage(jsonData)

	// Test 1: Simple base level extraction
	isee, err := Float(raw, "isee")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isee != 15000.50 {
		t.Errorf("expected 15000.50, got %v", isee)
	}

	// Test 2: Index-based array extraction
	corr0, err := Float(raw, "anni[0].corrispettivo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if corr0 != 318.0 {
		t.Errorf("expected 318.0, got %v", corr0)
	}

	corr1, err := Float(raw, "anni[1].corrispettivo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if corr1 != 450.5 {
		t.Errorf("expected 450.5, got %v", corr1)
	}

	// Test 3: Condition-based array extraction (=)
	corrCond1, err := Float(raw, "anni[annualita1=20232024].corrispettivo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if corrCond1 != 318.0 {
		t.Errorf("expected 318.0, got %v", corrCond1)
	}

	// Test 4: Condition-based array extraction (== and quotes)
	corrCond2, err := Float(raw, "anni[annualita1=='20242025'].corrispettivo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if corrCond2 != 450.5 {
		t.Errorf("expected 450.5, got %v", corrCond2)
	}

	// Test 5: String extraction from conditional array
	annVal, err := Str(raw, "anni[corrispettivo=450.5].annualita1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if annVal != "20242025" {
		t.Errorf("expected '20242025', got %v", annVal)
	}

	// Test 6: Expect error for out of bounds index
	_, err = Float(raw, "anni[2].corrispettivo")
	if err == nil {
		t.Errorf("expected error for index out of bounds, got nil")
	}

	// Test 7: Expect error for unsatisfied condition
	_, err = Float(raw, "anni[annualita1=20252026].corrispettivo")
	if err == nil {
		t.Errorf("expected error for unsatisfied condition, got nil")
	}
}
