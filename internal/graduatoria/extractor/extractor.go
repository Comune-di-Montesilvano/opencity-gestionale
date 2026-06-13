package extractor

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"opencity-gestionale/internal/opencity"
)

func resolveNode(data any, path string) (any, error) {
	if path == "" {
		return data, nil
	}
	parts := strings.SplitN(path, ".", 2)
	head, rest := parts[0], ""
	if len(parts) == 2 {
		rest = parts[1]
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
