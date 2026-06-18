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

	// Test 8: operatore != (diverso da)
	corrNeq, err := Float(raw, "anni[annualita1!=20232024].corrispettivo")
	if err != nil {
		t.Fatalf("!= unexpected error: %v", err)
	}
	if corrNeq != 450.5 {
		t.Errorf("!= expected 450.5, got %v", corrNeq)
	}

	// Test 9: operatore > (maggiore di) — verifica l'annualità dell'elemento con corrispettivo > 400
	annGt, err := Str(raw, "anni[corrispettivo>400].annualita1")
	if err != nil {
		t.Fatalf("> unexpected error: %v", err)
	}
	if annGt != "20242025" {
		t.Errorf("> expected '20242025', got %v", annGt)
	}

	// Test 10: operatore ~ (contiene)
	annTilde, err := Str(raw, "anni[annualita1~2024].annualita1")
	if err != nil {
		t.Fatalf("~ unexpected error: %v", err)
	}
	if annTilde != "20232024" {
		t.Errorf("~ expected '20232024', got %v", annTilde)
	}

	// Test 11: max: → elemento con corrispettivo massimo
	corrMax, err := Float(raw, "anni[max:corrispettivo].corrispettivo")
	if err != nil {
		t.Fatalf("max: unexpected error: %v", err)
	}
	if corrMax != 450.5 {
		t.Errorf("max: expected 450.5, got %v", corrMax)
	}

	// Test 12: min: → elemento con corrispettivo minimo
	corrMin, err := Float(raw, "anni[min:corrispettivo].corrispettivo")
	if err != nil {
		t.Fatalf("min: unexpected error: %v", err)
	}
	if corrMin != 318.0 {
		t.Errorf("min: expected 318.0, got %v", corrMin)
	}
}

func TestFlattenJSON_ArrayAsTerminal(t *testing.T) {
	raw := json.RawMessage(`{
		"nome": "Mario",
		"anni": [
			{"annualita": 2024, "importo": 100.5},
			{"annualita": 2025, "importo": 200.0}
		]
	}`)
	fields := FlattenJSON(raw)

	// "anni" deve apparire come nodo array terminale
	var anniField *FieldPreview
	for i := range fields {
		if fields[i].Path == "anni" {
			anniField = &fields[i]
		}
	}
	if anniField == nil {
		t.Fatal("manca nodo 'anni' nel risultato FlattenJSON")
	}
	if !anniField.IsArray {
		t.Error("campo 'anni' deve avere IsArray=true")
	}
	if len(anniField.ArraySampleKeys) == 0 {
		t.Error("ArraySampleKeys vuoto per 'anni'")
	}
	// Verifica che anni.0.annualita NON sia presente
	for _, f := range fields {
		if f.Path == "anni.0.annualita" || f.Path == "anni.0" {
			t.Errorf("path '%s' non deve essere presente quando IsArray=true", f.Path)
		}
	}
	// "nome" deve ancora essere presente
	var nomeFound bool
	for _, f := range fields {
		if f.Path == "nome" {
			nomeFound = true
		}
	}
	if !nomeFound {
		t.Error("campo 'nome' deve essere presente")
	}
}
