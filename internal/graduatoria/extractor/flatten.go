package extractor

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// FieldPreview rappresenta un campo del payload JSON con il suo percorso e valore.
type FieldPreview struct {
	Path  string
	Value string
}

// FlattenJSON appiattisce un json.RawMessage in una lista ordinata di path=valore.
// Per gli array espande al massimo i primi 2 elementi (indice 0 e 1).
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
const maxArrayElems = 2

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
		for i, elem := range val {
			if i >= maxArrayElems {
				break
			}
			key := fmt.Sprintf("%s.%d", prefix, i)
			if prefix == "" {
				key = fmt.Sprintf("%d", i)
			}
			flatten(elem, key, out, depth+1)
		}
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
