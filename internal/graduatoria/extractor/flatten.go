package extractor

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// FieldPreview rappresenta un campo del payload JSON con il suo percorso e valore.
type FieldPreview struct {
	Path            string
	Value           string
	IsArray         bool     // true = il nodo è un array, non espanso
	ArraySampleKeys []string // chiavi del primo elemento (disponibili per il builder UI)
}

// FlattenJSON appiattisce un json.RawMessage in una lista ordinata di path=valore.
// Gli array non vengono espansi: vengono emessi come nodi terminali con IsArray=true
// e ArraySampleKeys popolato con le chiavi del primo elemento.
func FlattenJSON(raw json.RawMessage) []FieldPreview {
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil
	}
	var out []FieldPreview
	flatten(root, "", &out, 0)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

const maxDepth = 8

func flatten(v any, prefix string, out *[]FieldPreview, depth int) {
	if depth > maxDepth {
		return
	}
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flatten(child, key, out, depth+1)
		}
	case []any:
		// Emette il nodo array senza espandere gli elementi
		fp := FieldPreview{
			Path:    prefix,
			Value:   fmt.Sprintf("[Array, %d elementi]", len(val)),
			IsArray: true,
		}
		// Raccoglie le chiavi del primo elemento per il builder
		if len(val) > 0 {
			if m, ok := val[0].(map[string]any); ok {
				for k := range m {
					fp.ArraySampleKeys = append(fp.ArraySampleKeys, k)
				}
				sort.Strings(fp.ArraySampleKeys)
			}
		}
		*out = append(*out, fp)
	case nil:
		*out = append(*out, FieldPreview{Path: prefix, Value: "(null)"})
	case bool:
		s := "false"
		if val {
			s = "true"
		}
		*out = append(*out, FieldPreview{Path: prefix, Value: s})
	case float64:
		// Mostra senza trailing zero se è intero
		if val == float64(int64(val)) {
			*out = append(*out, FieldPreview{Path: prefix, Value: fmt.Sprintf("%.0f", val)})
		} else {
			*out = append(*out, FieldPreview{Path: prefix, Value: fmt.Sprintf("%g", val)})
		}
	case string:
		s := val
		if len(s) > 80 {
			s = s[:80] + "…"
		}
		*out = append(*out, FieldPreview{Path: prefix, Value: s})
	default:
		*out = append(*out, FieldPreview{Path: prefix, Value: strings.TrimSpace(fmt.Sprintf("%v", val))})
	}
}
