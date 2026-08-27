package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	maxSchemaBytes  = 128 << 10
	maxPayloadBytes = 1 << 20
	maxJSONDepth    = 64
	maxJSONNodes    = 10000
)

type validationBudget struct {
	nodes int
}

func decodeJSON(raw []byte, limit int) (any, []byte, error) {
	if len(raw) == 0 || len(raw) > limit {
		return nil, nil, fmt.Errorf("JSON size must be between 1 and %d bytes", limit)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, nil, errors.New("JSON must contain one value")
		}
		return nil, nil, err
	}
	canonical, err := json.Marshal(value)
	return value, canonical, err
}

func validateSchema(raw []byte) (map[string]any, []byte, error) {
	value, canonical, err := decodeJSON(raw, maxSchemaBytes)
	if err != nil {
		return nil, nil, err
	}
	schema, ok := value.(map[string]any)
	if !ok {
		return nil, nil, errors.New("schema must be a JSON object")
	}
	budget := &validationBudget{}
	if err := inspectSchema(schema, 0, budget); err != nil {
		return nil, nil, err
	}
	return schema, canonical, nil
}

func inspectSchema(schema map[string]any, depth int, budget *validationBudget) error {
	if depth > maxJSONDepth {
		return errors.New("schema depth limit exceeded")
	}
	budget.nodes++
	if budget.nodes > maxJSONNodes {
		return errors.New("schema node limit exceeded")
	}
	if ref, ok := schema["$ref"].(string); ok && ref != "" {
		return errors.New("schema $ref is not supported in v1alpha1")
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		if len(properties) > 512 {
			return errors.New("schema property limit exceeded")
		}
		for name, rawChild := range properties {
			if strings.TrimSpace(name) == "" {
				return errors.New("schema property name must not be empty")
			}
			child, ok := rawChild.(map[string]any)
			if !ok {
				return fmt.Errorf("schema property %q must be an object", name)
			}
			if err := inspectSchema(child, depth+1, budget); err != nil {
				return fmt.Errorf("schema property %q: %w", name, err)
			}
		}
	}
	if rawItems, ok := schema["items"]; ok {
		items, ok := rawItems.(map[string]any)
		if !ok {
			return errors.New("schema items must be an object")
		}
		if err := inspectSchema(items, depth+1, budget); err != nil {
			return fmt.Errorf("schema items: %w", err)
		}
	}
	return nil
}

func validatePayload(schema map[string]any, raw []byte) ([]byte, error) {
	value, canonical, err := decodeJSON(raw, maxPayloadBytes)
	if err != nil {
		return nil, err
	}
	budget := &validationBudget{}
	if err := validateValue(schema, value, "$", 0, budget); err != nil {
		return nil, err
	}
	return canonical, nil
}

// ValidateAgainstSchema exposes the bounded v1alpha1 schema validator to
// trusted runtime stages without granting access to a registered type.
func ValidateAgainstSchema(schemaRaw, payloadRaw []byte) ([]byte, error) {
	schema, _, err := validateSchema(schemaRaw)
	if err != nil {
		return nil, err
	}
	return validatePayload(schema, payloadRaw)
}

func validateValue(schema map[string]any, value any, path string, depth int, budget *validationBudget) error {
	if depth > maxJSONDepth {
		return fmt.Errorf("%s exceeds JSON depth limit", path)
	}
	budget.nodes++
	if budget.nodes > maxJSONNodes {
		return errors.New("payload node limit exceeded")
	}
	if expected, _ := schema["type"].(string); expected != "" && !matchesType(expected, value) {
		return fmt.Errorf("%s must be %s", path, expected)
	}
	if enumValues, ok := schema["enum"].([]any); ok {
		actual, _ := json.Marshal(value)
		matched := false
		for _, enumValue := range enumValues {
			expected, _ := json.Marshal(enumValue)
			if bytes.Equal(actual, expected) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s is not an allowed enum value", path)
		}
	}
	switch typed := value.(type) {
	case map[string]any:
		if max, ok := numberAsInt(schema["maxProperties"]); ok && len(typed) > max {
			return fmt.Errorf("%s exceeds maxProperties", path)
		}
		required := map[string]bool{}
		if values, ok := schema["required"].([]any); ok {
			for _, item := range values {
				if name, ok := item.(string); ok {
					required[name] = true
				}
			}
		}
		for name := range required {
			if _, ok := typed[name]; !ok {
				return fmt.Errorf("%s.%s is required", path, name)
			}
		}
		properties, _ := schema["properties"].(map[string]any)
		additionalAllowed := true
		if allowed, ok := schema["additionalProperties"].(bool); ok {
			additionalAllowed = allowed
		}
		for name, childValue := range typed {
			rawChild, declared := properties[name]
			if !declared {
				if !additionalAllowed {
					return fmt.Errorf("%s.%s is not allowed", path, name)
				}
				continue
			}
			childSchema, _ := rawChild.(map[string]any)
			if err := validateValue(childSchema, childValue, path+"."+name, depth+1, budget); err != nil {
				return err
			}
		}
	case []any:
		if max, ok := numberAsInt(schema["maxItems"]); ok && len(typed) > max {
			return fmt.Errorf("%s exceeds maxItems", path)
		}
		if rawItems, ok := schema["items"].(map[string]any); ok {
			for index, item := range typed {
				if err := validateValue(rawItems, item, fmt.Sprintf("%s[%d]", path, index), depth+1, budget); err != nil {
					return err
				}
			}
		}
	case string:
		length := utf8.RuneCountInString(typed)
		if min, ok := numberAsInt(schema["minLength"]); ok && length < min {
			return fmt.Errorf("%s is shorter than minLength", path)
		}
		if max, ok := numberAsInt(schema["maxLength"]); ok && length > max {
			return fmt.Errorf("%s exceeds maxLength", path)
		}
	case json.Number:
		number, err := typed.Float64()
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return fmt.Errorf("%s is not a finite number", path)
		}
		if min, ok := numberAsFloat(schema["minimum"]); ok && number < min {
			return fmt.Errorf("%s is below minimum", path)
		}
		if max, ok := numberAsFloat(schema["maximum"]); ok && number > max {
			return fmt.Errorf("%s exceeds maximum", path)
		}
	}
	return nil
}

func matchesType(expected string, value any) bool {
	switch expected {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(json.Number)
		return ok
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		_, err := number.Int64()
		return err == nil
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	default:
		return false
	}
}

func numberAsFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		result, err := typed.Float64()
		return result, err == nil
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func numberAsInt(value any) (int, bool) {
	number, ok := numberAsFloat(value)
	if !ok || number < 0 || math.Trunc(number) != number || number > math.MaxInt32 {
		return 0, false
	}
	return int(number), true
}
