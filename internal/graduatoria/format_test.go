package graduatoria

import "testing"

func TestFormatFloatIT(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{1000000.00, "1.000.000,00"},
		{1234.56, "1.234,56"},
		{0.0, "0,00"},
		{0.5, "0,50"},
		{-1234.56, "-1.234,56"},
		{999.999, "1.000,00"}, // Arrotondamento
	}

	for _, tc := range tests {
		got := FormatFloatIT(tc.input)
		if got != tc.expected {
			t.Errorf("FormatFloatIT(%f) = %q; expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestFormatValutaIT(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{1000000.00, "1.000.000,00 €"},
		{1234.56, "1.234,56 €"},
	}

	for _, tc := range tests {
		got := FormatValutaIT(tc.input)
		if got != tc.expected {
			t.Errorf("FormatValutaIT(%f) = %q; expected %q", tc.input, got, tc.expected)
		}
	}
}
