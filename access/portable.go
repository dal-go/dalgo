package access

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExecutionGate restricts the execution surfaces at this policy's owner.
// A present gate with an empty allow list permits no query execution surface.
type ExecutionGate struct {
	Allow []ExecutionEntry `json:"allow" yaml:"allow"`
}
type ExecutionEntry struct {
	Class     ExecutionClass `json:"class" yaml:"class"`
	Namespace string         `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Mask      *Mask          `json:"mask,omitempty" yaml:"mask,omitempty"`
}
type ExecutionClass string

const (
	ExecutionTugQL           ExecutionClass = "tugql"
	ExecutionNativeSQL       ExecutionClass = "native_sql"
	ExecutionNativeGraphQL   ExecutionClass = "native_graphql"
	ExecutionStoredProcedure ExecutionClass = "stored_procedure"
)

// ParseTugQLPolicy validates a portable editable document. JSON is accepted as
// the JSON subset of YAML. Parsing does not activate policy or authorize data.
func ParseTugQLPolicy(data []byte) (TugQLDocument, error) {
	if len(data) > maxPolicyFileBytes {
		return TugQLDocument{}, fmt.Errorf("access: policy exceeds %d bytes", maxPolicyFileBytes)
	}
	if err := validatePolicySyntax(data, true); err != nil {
		return TugQLDocument{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var document TugQLDocument
	if err := decoder.Decode(&document); err != nil {
		return TugQLDocument{}, err
	}
	if err := ensureSingleDocument(func(v any) error { return decoder.Decode(v) }); err != nil {
		return TugQLDocument{}, err
	}
	return NormalizeTugQLPolicy(document)
}

// NormalizeTugQLPolicy returns a validated deep copy with canonical scoped
// masks. It preserves policy/rule identity and stage action-change order.
func NormalizeTugQLPolicy(document TugQLDocument) (TugQLDocument, error) {
	// Check presence before encoding: omitempty must not turn an explicitly
	// supplied empty fields list into an absent list beside a fieldMask.
	var checkFields func([]TugQLScope) error
	checkFields = func(scopes []TugQLScope) error {
		for _, scope := range scopes {
			for _, rule := range scope.Rules {
				if rule.FieldMask != nil && rule.Fields != nil {
					return fmt.Errorf("access: fields and fieldMask are mutually exclusive")
				}
			}
			if err := checkFields(scope.Scopes); err != nil {
				return err
			}
		}
		return nil
	}
	if err := checkFields(document.Scopes); err != nil {
		return TugQLDocument{}, err
	}
	for _, scopes := range document.RuleSets {
		if err := checkFields(scopes); err != nil {
			return TugQLDocument{}, err
		}
	}
	// Reusing the document's JSON shape supplies a defensive deep copy, including
	// row predicates and principal binding slices.
	data, err := json.Marshal(document)
	if err != nil {
		return TugQLDocument{}, err
	}
	// data was produced by json.Marshal immediately above and TugQLDocument has
	// no custom JSON decoder, so decoding the same shape cannot fail.
	result := decodeNormalizedTugQLPolicy(data)
	// Preserve integer precision in untyped predicate constants.
	var restore func(any) (any, error)
	restore = func(value any) (any, error) {
		switch v := value.(type) {
		case json.Number:
			if !strings.ContainsAny(string(v), ".eE") {
				if n, e := strconv.ParseInt(string(v), 10, 64); e == nil {
					return n, nil
				}
				if n, e := strconv.ParseUint(string(v), 10, 64); e == nil {
					return n, nil
				}
				return nil, fmt.Errorf("access: integer constant outside supported range")
			}
			return strconv.ParseFloat(string(v), 64)
		case []any:
			for i := range v {
				var e error
				v[i], e = restore(v[i])
				if e != nil {
					return nil, e
				}
			}
		case map[string]any:
			for k := range v {
				var e error
				v[k], e = restore(v[k])
				if e != nil {
					return nil, e
				}
			}
		}
		return value, nil
	}
	var restoreCondition func(*DocumentCondition) error
	restoreCondition = func(c *DocumentCondition) error {
		if c == nil {
			return nil
		}
		for _, expr := range []*DocumentExpression{c.Left, c.Right} {
			if expr == nil {
				continue
			}
			for _, value := range []*any{&expr.Value, &expr.Values} {
				restored, e := restore(*value)
				if e != nil {
					return e
				}
				*value = restored
			}
		}
		for i := range c.And {
			if e := restoreCondition(&c.And[i]); e != nil {
				return e
			}
		}
		for i := range c.Or {
			if e := restoreCondition(&c.Or[i]); e != nil {
				return e
			}
		}
		return nil
	}
	var restoreScopes func([]TugQLScope) error
	restoreScopes = func(scopes []TugQLScope) error {
		for i := range scopes {
			for j := range scopes[i].Rules {
				r := &scopes[i].Rules[j]
				if e := restoreCondition(r.Where); e != nil {
					return e
				}
				if e := restoreCondition(r.Check); e != nil {
					return e
				}
			}
			if e := restoreScopes(scopes[i].Scopes); e != nil {
				return e
			}
		}
		return nil
	}
	if err = restoreScopes(result.Scopes); err != nil {
		return TugQLDocument{}, err
	}
	for _, scopes := range result.RuleSets {
		if err = restoreScopes(scopes); err != nil {
			return TugQLDocument{}, err
		}
	}
	if strings.TrimSpace(result.Target.Database) == "" {
		return TugQLDocument{}, fmt.Errorf("access: target.database is required")
	}
	if result.Metadata.Visibility == "" {
		result.Metadata.Visibility = "public"
	}
	stages, patterns := 0, 0
	normalize := func(mask **Mask, kind MaskKind) error {
		if *mask == nil {
			return nil
		}
		stages += len((*mask).Stages)
		for _, stage := range (*mask).Stages {
			patterns += len(stage.Include) + len(stage.Exclude)
		}
		if stages > MaxMaskStages || patterns > MaxMaskPatterns {
			return fmt.Errorf("access: policy exceeds raw mask limits")
		}
		compiled, err := CompileMask(**mask, kind)
		if err != nil {
			return err
		}
		canonical := compiled.Canonical()
		*mask = &canonical
		return nil
	}
	if err = normalize(&result.CollectionMask, NameMask); err != nil {
		return TugQLDocument{}, err
	}
	var scopes func([]TugQLScope) error
	scopes = func(items []TugQLScope) error {
		for i := range items {
			scope := &items[i]
			if scope.CollectionMask != nil {
				return fmt.Errorf("access: collectionMask belongs at policy level")
			}
			for j := range scope.Rules {
				rule := &scope.Rules[j]
				if rule.FieldMask != nil {
					if rule.Fields != nil {
						return fmt.Errorf("access: fields and fieldMask are mutually exclusive")
					}
					if rule.Effect != "allow" {
						return fmt.Errorf("access: fieldMask is valid only on allow rules")
					}
					if err := normalize(&rule.FieldMask, FieldMask); err != nil {
						return err
					}
				}
			}
			if err := scopes(scope.Scopes); err != nil {
				return err
			}
		}
		return nil
	}
	if err = scopes(result.Scopes); err != nil {
		return TugQLDocument{}, err
	}
	for _, items := range result.RuleSets {
		if err = scopes(items); err != nil {
			return TugQLDocument{}, err
		}
	}
	if gate := result.Execution; gate != nil {
		if gate.Allow == nil || len(gate.Allow) > 32 {
			return TugQLDocument{}, fmt.Errorf("access: execution.allow requires an array of at most 32 entries")
		}
		seen := map[string]bool{}
		for i := range gate.Allow {
			entry := &gate.Allow[i]
			key := string(entry.Class) + "\x00" + entry.Namespace
			if seen[key] {
				return TugQLDocument{}, fmt.Errorf("access: duplicate execution class/namespace")
			}
			seen[key] = true
			switch entry.Class {
			case ExecutionTugQL, ExecutionNativeSQL, ExecutionNativeGraphQL:
				if entry.Namespace != "" || entry.Mask != nil {
					return TugQLDocument{}, fmt.Errorf("access: only procedure entries have namespace and mask")
				}
			case ExecutionStoredProcedure:
				if entry.Mask == nil || entry.Namespace == "" || strings.ContainsAny(entry.Namespace, "*./\\") {
					return TugQLDocument{}, fmt.Errorf("access: procedure requires a literal namespace and mask")
				}
				if _, err := normalizeMaskPattern(entry.Namespace, NameMask); err != nil {
					return TugQLDocument{}, err
				}
				if err := normalize(&entry.Mask, NameMask); err != nil {
					return TugQLDocument{}, err
				}
			default:
				return TugQLDocument{}, fmt.Errorf("access: unsupported execution class %q", entry.Class)
			}
		}
	}
	// Validate ordinary row/action/scope/principal semantics using the existing
	// DALgo document compiler. Strip only the extensions just validated above.
	baseline := result
	baseline.CollectionMask = nil
	baseline.Execution = nil
	var clearMasks func([]TugQLScope) []TugQLScope
	clearMasks = func(items []TugQLScope) []TugQLScope {
		copied := append([]TugQLScope(nil), items...)
		for i := range copied {
			copied[i].Rules = append([]TugQLRule(nil), items[i].Rules...)
			for j := range copied[i].Rules {
				copied[i].Rules[j].FieldMask = nil
			}
			copied[i].Scopes = clearMasks(items[i].Scopes)
		}
		return copied
	}
	baseline.Scopes = clearMasks(result.Scopes)
	if result.RuleSets != nil {
		baseline.RuleSets = map[string][]TugQLScope{}
		for name, items := range result.RuleSets {
			baseline.RuleSets[name] = clearMasks(items)
		}
	}
	if _, err = policyFromTugQLDocument(baseline, baseline.Target.Database, ""); err != nil {
		return TugQLDocument{}, err
	}
	return result, nil
}

func decodeNormalizedTugQLPolicy(data []byte) (result TugQLDocument) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	_ = decoder.Decode(&result)
	return
}

// MarshalTugQLPolicyYAML emits normalized policy text. Formatting and comments
// from the original document are deliberately not preserved in this version.
func MarshalTugQLPolicyYAML(document TugQLDocument) ([]byte, error) {
	var output bytes.Buffer
	if err := writeTugQLPolicyYAML(&output, document); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
func writeTugQLPolicyYAML(writer io.Writer, document TugQLDocument) error {
	normalized, err := NormalizeTugQLPolicy(document)
	if err != nil {
		return err
	}
	encoder := yaml.NewEncoder(writer)
	encoder.SetIndent(2)
	if err = encoder.Encode(normalized); err != nil {
		return err
	}
	return encoder.Close()
}
func MarshalTugQLPolicyJSON(document TugQLDocument) ([]byte, error) {
	normalized, err := NormalizeTugQLPolicy(document)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(normalized, "", "  ")
}

// Explicit null is not absence for security-bearing selectors. The typed
// decoders otherwise collapse null pointers and empty collections.
func validateTugQLShape(root *yaml.Node) error {
	mask := func(node *yaml.Node) error {
		if node == nil {
			return nil
		}
		if node.Kind != yaml.MappingNode {
			return fmt.Errorf("access: mask requires an object")
		}
		stages := mappingValue(node, "stages")
		if stages == nil || stages.Kind != yaml.SequenceNode {
			return fmt.Errorf("access: mask.stages requires an array")
		}
		for _, stage := range stages.Content {
			if stage.Kind != yaml.MappingNode || len(stage.Content) != 2 {
				return fmt.Errorf("access: mask stage requires exactly one action")
			}
			action, list := stage.Content[0].Value, stage.Content[1]
			if (action != "include" && action != "exclude") || list.Kind != yaml.SequenceNode {
				return fmt.Errorf("access: mask stage requires include or exclude array")
			}
		}
		return nil
	}
	if err := mask(mappingValue(root, "collectionMask")); err != nil {
		return err
	}
	var scopes func(*yaml.Node) error
	scopes = func(node *yaml.Node) error {
		if node == nil {
			return nil
		}
		for _, scope := range node.Content {
			if mappingValue(scope, "collectionMask") != nil {
				return fmt.Errorf("access: collectionMask belongs at policy level")
			}
			if rules := mappingValue(scope, "rules"); rules != nil {
				for _, rule := range rules.Content {
					fieldMask := mappingValue(rule, "fieldMask")
					if fieldMask != nil && mappingValue(rule, "fields") != nil {
						return fmt.Errorf("access: fields and fieldMask are mutually exclusive")
					}
					if err := mask(fieldMask); err != nil {
						return err
					}
				}
			}
			if err := scopes(mappingValue(scope, "scopes")); err != nil {
				return err
			}
		}
		return nil
	}
	if err := scopes(mappingValue(root, "scopes")); err != nil {
		return err
	}
	if sets := mappingValue(root, "ruleSets"); sets != nil {
		for i := 1; i < len(sets.Content); i += 2 {
			if err := scopes(sets.Content[i]); err != nil {
				return err
			}
		}
	}
	if gate := mappingValue(root, "execution"); gate != nil {
		allow := mappingValue(gate, "allow")
		if gate.Kind != yaml.MappingNode || allow == nil || allow.Kind != yaml.SequenceNode {
			return fmt.Errorf("access: execution.allow requires an array")
		}
		for _, entry := range allow.Content {
			if err := mask(mappingValue(entry, "mask")); err != nil {
				return err
			}
		}
	}
	return nil
}
