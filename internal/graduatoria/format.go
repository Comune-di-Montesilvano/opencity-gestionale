package graduatoria

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatFloatIT formatta un float64 con il separatore delle migliaia '.' e dei decimali ',' (es. 1.000.000,00)
func FormatFloatIT(val float64) string {
	sign := ""
	if val < 0 {
		sign = "-"
		val = -val
	}
	// Arrotonda a 2 decimali per la visualizzazione
	intPart := int64(val)
	decPart := int64((val-float64(intPart))*100 + 0.5)
	if decPart >= 100 {
		intPart++
		decPart -= 100
	}

	// Formatta la parte intera con i punti delle migliaia
	s := strconv.FormatInt(intPart, 10)
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	if len(s) > 0 {
		parts = append([]string{s}, parts...)
	}
	formattedInt := strings.Join(parts, ".")

	return fmt.Sprintf("%s%s,%02d", sign, formattedInt, decPart)
}

// FormatValutaIT formatta un float64 come valuta in Euro (es. 1.000.000,00 €)
func FormatValutaIT(val float64) string {
	return FormatFloatIT(val) + " €"
}
