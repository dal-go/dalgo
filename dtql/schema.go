package dtql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// SchemaID is the canonical $id of the published DTQL JSON Schema.
const SchemaID = "https://dal-go.github.io/dtql/schema.json"

// schemaDocument builds the JSON Schema (draft 2020-12) describing the in-scope
// DTQL-YAML, derived from the dtql Go shape types and the in-scope operator set
// (the single source of truth). It is rendered to schema.json and schema.yaml by
// the gen-schema command and re-derived by the freshness test, so the committed
// schema cannot drift from the types.
func schemaDocument() map[string]any {
	scalar := map[string]any{"type": []any{"string", "number", "boolean"}}
	exprProps := map[string]any{
		"field":  map[string]any{"type": "string"},
		"value":  scalar,
		"values": map[string]any{"type": "array", "items": scalar},
	}
	exprOneOf := []any{
		map[string]any{"required": []any{"field"}},
		map[string]any{"required": []any{"value"}},
		map[string]any{"required": []any{"values"}},
	}
	// A column/order is an expression plus one extra key (as / desc).
	columnProps := mergeProps(exprProps, map[string]any{"as": map[string]any{"type": "string"}})
	orderProps := mergeProps(exprProps, map[string]any{"desc": map[string]any{"type": "boolean"}})

	defs := map[string]any{
		"from": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"name"},
			"properties": map[string]any{
				"name":  map[string]any{"type": "string", "minLength": 1},
				"alias": map[string]any{"type": "string"},
			},
		},
		"expression": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           exprProps,
			"oneOf":                exprOneOf,
		},
		"column": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           columnProps,
			"oneOf":                exprOneOf,
		},
		"order": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           orderProps,
			"oneOf":                exprOneOf,
		},
		"comparison": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"op", "left", "right"},
			"properties": map[string]any{
				"op":    map[string]any{"enum": comparisonOpEnum()},
				"left":  map[string]any{"$ref": "#/$defs/expression"},
				"right": map[string]any{"$ref": "#/$defs/expression"},
			},
		},
		"group": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"and": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/condition"}},
				"or":  map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/condition"}},
			},
			"oneOf": []any{
				map[string]any{"required": []any{"and"}},
				map[string]any{"required": []any{"or"}},
			},
		},
		"condition": map[string]any{
			"oneOf": []any{
				map[string]any{"$ref": "#/$defs/comparison"},
				map[string]any{"$ref": "#/$defs/group"},
			},
		},
	}

	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  SchemaID,
		"title":                "DTQL",
		"description":          "YAML serialization of dalgo's dal.StructuredQuery (core relational read-only subset).",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"from"},
		"properties": map[string]any{
			"from":    map[string]any{"$ref": "#/$defs/from"},
			"columns": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/column"}},
			"where":   map[string]any{"$ref": "#/$defs/condition"},
			"orderBy": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/order"}},
			"limit":   map[string]any{"type": "integer", "minimum": 0},
			"offset":  map[string]any{"type": "integer", "minimum": 0},
		},
		"$defs": defs,
	}
}

func mergeProps(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// comparisonOpEnum returns the in-scope comparison operators (sorted) as the
// schema enum, derived from inScopeComparisonOps so the schema tracks the code.
func comparisonOpEnum() []any {
	ops := make([]string, 0, len(inScopeComparisonOps))
	for op := range inScopeComparisonOps {
		ops = append(ops, string(op))
	}
	sort.Strings(ops)
	enum := make([]any, len(ops))
	for i, op := range ops {
		enum[i] = op
	}
	return enum
}

// SchemaJSON renders the DTQL JSON Schema as canonical, indented JSON.
func SchemaJSON() ([]byte, error) {
	b, err := json.MarshalIndent(schemaDocument(), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal schema JSON: %w", err)
	}
	return append(b, '\n'), nil
}

// SchemaYAML renders the DTQL JSON Schema as canonical YAML (identical content).
func SchemaYAML() ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(schemaDocument()); err != nil {
		return nil, fmt.Errorf("marshal schema YAML: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("finalize schema YAML: %w", err)
	}
	return buf.Bytes(), nil
}
