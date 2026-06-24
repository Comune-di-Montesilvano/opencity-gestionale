package extractor_test

import (
	"encoding/json"
	"testing"
	"opencity-gestionale/internal/graduatoria/extractor"
)

func TestFlattenArrayFirstValuesFloat(t *testing.T) {
	raw := json.RawMessage(`{"anni":[{"annualita1":20232024,"corrispettivo":100.5}]}`)
	fields := extractor.FlattenJSON(raw)
	var anniField *extractor.FieldPreview
	for i := range fields {
		if fields[i].Path == "anni" {
			anniField = &fields[i]
			break
		}
	}
	if anniField == nil {
		t.Fatal("campo anni non trovato")
	}
	got := anniField.ArrayFirstValues["annualita1"]
	if got != "20232024" {
		t.Errorf("ArrayFirstValues[annualita1] = %q, want %q", got, "20232024")
	}
	got2 := anniField.ArrayFirstValues["corrispettivo"]
	if got2 != "100.5" {
		t.Errorf("ArrayFirstValues[corrispettivo] = %q, want %q", got2, "100.5")
	}
}
