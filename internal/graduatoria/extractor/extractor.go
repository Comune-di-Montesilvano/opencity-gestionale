package extractor

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"opencity-gestionale/internal/opencity"
)

func resolveNode(data any, path string) (any, error) {
	if path == "" {
		return data, nil
	}

	// Split by first '.' that is not inside '[' and ']'
	head, rest := "", ""
	inBrackets := 0
	foundDot := false
	for i, char := range path {
		if char == '[' {
			inBrackets++
		} else if char == ']' {
			inBrackets--
		} else if char == '.' && inBrackets == 0 {
			head = path[:i]
			rest = path[i+1:]
			foundDot = true
			break
		}
	}
	if !foundDot {
		head = path
	}

	// Support conditional indexing like: anni[annualita1=20232024] or anni[0]
	if strings.Contains(head, "[") && strings.HasSuffix(head, "]") {
		idxOpen := strings.Index(head, "[")
		arrayKey := head[:idxOpen]
		cond := head[idxOpen+1 : len(head)-1]

		m, ok := data.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("atteso oggetto a %q, trovato %T", arrayKey, data)
		}
		val, ok := m[arrayKey]
		if !ok {
			return nil, fmt.Errorf("chiave %q non trovata", arrayKey)
		}

		arr, ok := val.([]any)
		if !ok {
			return nil, fmt.Errorf("atteso array a %q, trovato %T", arrayKey, val)
		}

		// check if condition is a pure index (all digits)
		isIndex := true
		for _, char := range cond {
			if char < '0' || char > '9' {
				isIndex = false
				break
			}
		}

		var item any
		if isIndex {
			var idx int
			_, err := fmt.Sscanf(cond, "%d", &idx)
			if err != nil {
				return nil, fmt.Errorf("indice non valido %q in %q: %w", cond, head, err)
			}
			if idx < 0 || idx >= len(arr) {
				return nil, fmt.Errorf("indice %d fuori dai limiti [0-%d] in %q", idx, len(arr)-1, head)
			}
			item = arr[idx]
		} else if strings.HasPrefix(cond, "max:") || strings.HasPrefix(cond, "min:") {
			// Aggregazione: max:campo o min:campo → elemento con valore massimo/minimo
			isMax := strings.HasPrefix(cond, "max:")
			aggKey := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(cond, "max:"), "min:"))
			bestIdx := -1
			var bestVal float64
			for i, elem := range arr {
				vNode, err := resolveNode(elem, aggKey)
				if err != nil {
					continue
				}
				var f float64
				switch n := vNode.(type) {
				case float64:
					f = n
				case json.Number:
					if ff, err := n.Float64(); err == nil {
						f = ff
					} else {
						continue
					}
				case string:
					ff, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
					if err != nil {
						continue
					}
					f = ff
				default:
					continue
				}
				if bestIdx == -1 || (isMax && f > bestVal) || (!isMax && f < bestVal) {
					bestIdx = i
					bestVal = f
				}
			}
			if bestIdx == -1 {
				return nil, fmt.Errorf("nessun elemento con campo %q in %q", aggKey, arrayKey)
			}
			item = arr[bestIdx]
		} else {
			// Condizione comparativa: key op val
			// Tenta operatori in ordine di specificità (multi-char prima di single-char)
			var condKey, condOp, condVal string
			for _, op := range []string{"!=", ">=", "<=", "~", ">", "<", "==", "="} {
				idx := strings.Index(cond, op)
				if idx >= 0 {
					condKey = strings.TrimSpace(cond[:idx])
					condOp = op
					condVal = strings.Trim(strings.TrimSpace(cond[idx+len(op):]), `"'`)
					break
				}
			}
			if condKey == "" {
				return nil, fmt.Errorf("condizione non valida %q in %q", cond, head)
			}

			var found any
			for _, elem := range arr {
				vKey, err := resolveNode(elem, condKey)
				if err != nil {
					continue
				}
				var vStr string
				if f, ok := vKey.(float64); ok {
					vStr = strconv.FormatFloat(f, 'f', -1, 64)
				} else {
					vStr = strings.TrimSpace(fmt.Sprintf("%v", vKey))
				}
				var matches bool
				switch condOp {
				case "=", "==":
					matches = vStr == condVal
				case "!=":
					matches = vStr != condVal
				case "~":
					matches = strings.Contains(vStr, condVal)
				case ">", "<", ">=", "<=":
					vF, err1 := strconv.ParseFloat(vStr, 64)
					cF, err2 := strconv.ParseFloat(condVal, 64)
					if err1 != nil || err2 != nil {
						continue
					}
					switch condOp {
					case ">":
						matches = vF > cF
					case "<":
						matches = vF < cF
					case ">=":
						matches = vF >= cF
					case "<=":
						matches = vF <= cF
					}
				}
				if matches {
					found = elem
					break
				}
			}
			if found == nil {
				return nil, fmt.Errorf("nessun elemento soddisfa la condizione %q in %q", cond, head)
			}
			item = found
		}

		if rest == "" {
			return item, nil
		}
		return resolveNode(item, rest)
	}

	m, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("atteso oggetto a %q, trovato %T", head, data)
	}
	val, ok := m[head]
	if !ok {
		return nil, fmt.Errorf("chiave %q non trovata", head)
	}
	if rest == "" {
		return val, nil
	}
	return resolveNode(val, rest)
}

func parse(raw json.RawMessage) (any, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func Float(data json.RawMessage, path string) (float64, error) {
	v, err := parse(data)
	if err != nil {
		return 0, err
	}
	node, err := resolveNode(v, path)
	if err != nil {
		return 0, err
	}
	switch n := node.(type) {
	case float64:
		return n, nil
	case json.Number:
		return n.Float64()
	case nil:
		return 0, nil
	}
	return 0, fmt.Errorf("path %q: atteso numero, trovato %T", path, node)
}

func Str(data json.RawMessage, path string) (string, error) {
	v, err := parse(data)
	if err != nil {
		return "", err
	}
	node, err := resolveNode(v, path)
	if err != nil {
		return "", err
	}
	if node == nil {
		return "", nil
	}
	if s, ok := node.(string); ok {
		return s, nil
	}
	return fmt.Sprintf("%v", node), nil
}

func Count(data json.RawMessage, path string) (int, error) {
	v, err := parse(data)
	if err != nil {
		return 0, err
	}
	node, err := resolveNode(v, path)
	if err != nil {
		return 0, err
	}
	arr, ok := node.([]any)
	if !ok {
		return 0, fmt.Errorf("path %q: atteso array, trovato %T", path, node)
	}
	return len(arr), nil
}

var timeLayouts = []string{time.RFC3339, "2006-01-02", "02/01/2006"}

func Time(data json.RawMessage, path string) (time.Time, error) {
	s, err := Str(data, path)
	if err != nil {
		return time.Time{}, err
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("path %q: impossibile parsare %q come data", path, s)
}

func ArrayElements(data json.RawMessage, path string) ([]json.RawMessage, error) {
	v, err := parse(data)
	if err != nil {
		return nil, err
	}
	// Filter-mode: "arrayKey[condition]" senza navigazione dopo → ritorna TUTTI i matching.
	if filtered, ok, err2 := tryFilterArray(v, path); ok {
		return filtered, err2
	}
	node, err := resolveNode(v, path)
	if err != nil {
		return nil, err
	}
	arr, ok := node.([]any)
	if !ok {
		return nil, fmt.Errorf("path %q: atteso array, trovato %T", path, node)
	}
	out := make([]json.RawMessage, len(arr))
	for i, elem := range arr {
		b, err := json.Marshal(elem)
		if err != nil {
			return nil, err
		}
		out[i] = b
	}
	return out, nil
}

// tryFilterArray gestisce il pattern "arrayKey[cond]" (senza '.' dopo il ']' finale).
// Ritorna tutti gli elementi dell'array che soddisfano la condizione.
// Ritorna (nil, false, nil) se il pattern non è rilevato (delegare a resolveNode).
func tryFilterArray(v any, path string) ([]json.RawMessage, bool, error) {
	idxOpen := strings.Index(path, "[")
	if idxOpen <= 0 || !strings.HasSuffix(path, "]") {
		return nil, false, nil
	}
	arrayKey := path[:idxOpen]
	cond := path[idxOpen+1 : len(path)-1]

	// Non attivare per indici numerici o aggregazioni max:/min:
	if allDigits(cond) || strings.HasPrefix(cond, "max:") || strings.HasPrefix(cond, "min:") {
		return nil, false, nil
	}

	condKey, condOp, condVal, ok := parseCond(cond)
	if !ok {
		return nil, false, nil
	}

	m, mOk := v.(map[string]any)
	if !mOk {
		return nil, true, fmt.Errorf("atteso oggetto per chiave %q", arrayKey)
	}
	val, exists := m[arrayKey]
	if !exists {
		return nil, true, fmt.Errorf("chiave %q non trovata", arrayKey)
	}
	arr, aOk := val.([]any)
	if !aOk {
		return nil, true, fmt.Errorf("atteso array a %q, trovato %T", arrayKey, val)
	}

	var out []json.RawMessage
	for _, elem := range arr {
		vKey, err := resolveNode(elem, condKey)
		if err != nil {
			continue
		}
		if matchesCond(vKey, condOp, condVal) {
			b, _ := json.Marshal(elem)
			out = append(out, b)
		}
	}
	return out, true, nil
}

// parseCond estrae condKey, condOp, condVal da una stringa condizione come "annualita1==20242025".
func parseCond(cond string) (key, op, val string, ok bool) {
	for _, o := range []string{"!=", ">=", "<=", "~", ">", "<", "==", "="} {
		idx := strings.Index(cond, o)
		if idx >= 0 {
			return strings.TrimSpace(cond[:idx]), o,
				strings.Trim(strings.TrimSpace(cond[idx+len(o):]), `"'`), true
		}
	}
	return "", "", "", false
}

// matchesCond confronta vKey con condVal usando condOp.
func matchesCond(vKey any, condOp, condVal string) bool {
	var vStr string
	if f, ok := vKey.(float64); ok {
		vStr = strconv.FormatFloat(f, 'f', -1, 64)
	} else {
		vStr = strings.TrimSpace(fmt.Sprintf("%v", vKey))
	}
	switch condOp {
	case "=", "==":
		return vStr == condVal
	case "!=":
		return vStr != condVal
	case "~":
		return strings.Contains(vStr, condVal)
	case ">", "<", ">=", "<=":
		vF, err1 := strconv.ParseFloat(vStr, 64)
		cF, err2 := strconv.ParseFloat(condVal, 64)
		if err1 != nil || err2 != nil {
			return false
		}
		switch condOp {
		case ">":
			return vF > cF
		case "<":
			return vF < cF
		case ">=":
			return vF >= cF
		case "<=":
			return vF <= cF
		}
	}
	return false
}

// allDigits ritorna true se s è composta solo di cifre (0-9).
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func AppField(app opencity.Application, field string) (string, error) {
	switch field {
	case "submitted_at":
		return app.SubmittedAt, nil
	case "id":
		return app.ID, nil
	case "protocol_number":
		return app.ProtocolNumber, nil
	case "status":
		return app.Status, nil
	case "status_name":
		return app.StatusName, nil
	}
	return "", fmt.Errorf("campo app sconosciuto %q", field)
}
