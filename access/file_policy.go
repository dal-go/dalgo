package access

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DTQLDocumentAPIVersion = "dtql.org/access/v1"
	maxPolicyFileBytes     = 1 << 20
)

// FilePolicyConfig selects portable policy files relative to a trusted root.
// Loading is opt-in: a zero value disables file policies.
type FilePolicyConfig struct {
	Enabled  bool     `json:"enabled" yaml:"enabled"`
	Database string   `json:"database" yaml:"database"`
	Policies []string `json:"policies" yaml:"policies"`
}

type DTQLDocument struct {
	APIVersion     string                 `json:"apiVersion" yaml:"apiVersion"`
	Kind           string                 `json:"kind" yaml:"kind"`
	Metadata       DTQLMetadata           `json:"metadata" yaml:"metadata"`
	Target         DTQLTarget             `json:"target" yaml:"target"`
	Composition    string                 `json:"composition" yaml:"composition"`
	Default        string                 `json:"default" yaml:"default"`
	Scopes         []DTQLScope            `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	RuleSets       map[string][]DTQLScope `json:"ruleSets,omitempty" yaml:"ruleSets,omitempty"`
	Bindings       *DocumentBindings      `json:"bindings,omitempty" yaml:"bindings,omitempty"`
	Execution      *ExecutionGate         `json:"execution,omitempty" yaml:"execution,omitempty"`
	CollectionMask *Mask                  `json:"collectionMask,omitempty" yaml:"collectionMask,omitempty"`
}

type DTQLMetadata struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty" yaml:"visibility,omitempty"`
}

type DTQLTarget struct {
	Database string `json:"database" yaml:"database"`
}

type DTQLScope struct {
	Path            string      `json:"path,omitempty" yaml:"path,omitempty"`
	CollectionGroup string      `json:"collectionGroup,omitempty" yaml:"collectionGroup,omitempty"`
	OpaqueQuery     bool        `json:"opaqueQuery,omitempty" yaml:"opaqueQuery,omitempty"`
	Rules           []DTQLRule  `json:"rules,omitempty" yaml:"rules,omitempty"`
	Scopes          []DTQLScope `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	CollectionMask  *Mask       `json:"collectionMask,omitempty" yaml:"collectionMask,omitempty"`
}

type DTQLRule struct {
	ID         string             `json:"id" yaml:"id"`
	Effect     string             `json:"effect" yaml:"effect"`
	Operations []string           `json:"operations" yaml:"operations"`
	Where      *DocumentCondition `json:"where,omitempty" yaml:"where,omitempty"`
	Check      *DocumentCondition `json:"check,omitempty" yaml:"check,omitempty"`
	Fields     []string           `json:"fields,omitempty" yaml:"fields,omitempty"`
	FieldMask  *Mask              `json:"fieldMask,omitempty" yaml:"fieldMask,omitempty"`
}

// LoadPolicyFiles loads enabled DTQL portable policy documents from root.
// Every configured file must be valid; errors fail the complete load closed.
func LoadPolicyFiles(root string, config FilePolicyConfig) ([]Policy, error) {
	if !config.Enabled {
		return nil, nil
	}
	if strings.TrimSpace(config.Database) == "" {
		return nil, fmt.Errorf("access: enabled file policies require a database")
	}
	if len(config.Policies) == 0 {
		return nil, fmt.Errorf("access: enabled file policies require at least one policy file")
	}
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("access: open policy root: %w", err)
	}
	defer rootFS.Close()

	policies := make([]Policy, 0, len(config.Policies))
	names := make(map[string]string, len(config.Policies))
	for _, name := range config.Policies {
		policy, err := loadPolicyFile(rootFS, name, config.Database)
		if err != nil {
			return nil, err
		}
		if previous, exists := names[policy.Name()]; exists {
			return nil, fmt.Errorf("access: duplicate policy id %q in %q and %q", policy.Name(), previous, name)
		}
		names[policy.Name()] = name
		policies = append(policies, policy)
	}
	return policies, nil
}

func loadPolicyFile(root *os.Root, name, database string) (Policy, error) {
	if name == "" || filepath.IsAbs(name) {
		return nil, fmt.Errorf("access: invalid policy file %q", name)
	}
	before, err := root.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("access: inspect policy file %q: %w", name, err)
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("access: policy file %q must not be a symbolic link", name)
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("access: policy file %q is not a regular file", name)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("access: open policy file %q: %w", name, err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("access: inspect open policy file %q: %w", name, err)
	}
	after, err := root.Lstat(name)
	if err != nil || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, opened) || !os.SameFile(opened, after) {
		return nil, fmt.Errorf("access: policy file %q changed while opening", name)
	}
	if !opened.Mode().IsRegular() {
		return nil, fmt.Errorf("access: policy file %q is not a regular file", name)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxPolicyFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("access: read policy file %q: %w", name, err)
	}
	if len(data) > maxPolicyFileBytes {
		return nil, fmt.Errorf("access: policy file %q exceeds %d bytes", name, maxPolicyFileBytes)
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json":
		if !json.Valid(data) {
			return nil, fmt.Errorf("access: invalid JSON policy file %q", name)
		}
	case ".yaml", ".yml":
	default:
		return nil, fmt.Errorf("access: policy file %q must use .yaml, .yml, or .json", name)
	}
	document, err := ParseDTQLPolicy(data)
	if err != nil {
		return nil, fmt.Errorf("access: decode policy file %q: %w", name, err)
	}

	return policyFromDTQLDocument(document, database, name)
}

func validatePortablePolicyYAML(data []byte) error { return validatePolicySyntax(data, false) }

func validatePolicySyntax(data []byte, allowExtensions bool) error {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil // the format-specific strict decoder reports syntax errors
	}
	var validateMappings func(*yaml.Node) error
	validateMappings = func(node *yaml.Node) error {
		if node.Kind == yaml.AliasNode {
			return fmt.Errorf("YAML aliases are not allowed in portable policies")
		}
		if node.Kind == yaml.MappingNode {
			seen := make(map[string]struct{}, len(node.Content)/2)
			for i := 0; i+1 < len(node.Content); i += 2 {
				key := node.Content[i]
				if key.Value == "<<" || key.Tag == "!!merge" {
					return fmt.Errorf("YAML merge keys are not allowed in portable policies")
				}
				identity := key.Tag + "\x00" + key.Value
				if _, exists := seen[identity]; exists {
					return fmt.Errorf("duplicate mapping key %q", key.Value)
				}
				seen[identity] = struct{}{}
				if err := validateMappings(node.Content[i+1]); err != nil {
					return err
				}
			}
		} else {
			for _, child := range node.Content {
				if err := validateMappings(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := validateMappings(&document); err != nil {
		return err
	}
	if len(document.Content) == 0 {
		return nil
	}
	root := document.Content[0]
	if allowExtensions {
		return validateDTQLShape(root)
	}
	if feature := mappingFeature(root, "execution", "collectionMask"); feature != "" {
		return fmt.Errorf("%s is not supported by this loader", feature)
	}
	if scopes := mappingValue(root, "scopes"); scopes != nil {
		if feature := scopedPolicyFeature(scopes); feature != "" {
			return fmt.Errorf("%s is not supported by this loader", feature)
		}
	}
	if ruleSets := mappingValue(root, "ruleSets"); ruleSets != nil && ruleSets.Kind == yaml.MappingNode {
		for i := 1; i < len(ruleSets.Content); i += 2 {
			if feature := scopedPolicyFeature(ruleSets.Content[i]); feature != "" {
				return fmt.Errorf("%s is not supported by this loader", feature)
			}
		}
	}
	return nil
}

func mappingValue(node *yaml.Node, name string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == name {
			return node.Content[i+1]
		}
	}
	return nil
}

func mappingFeature(node *yaml.Node, names ...string) string {
	for _, name := range names {
		if mappingValue(node, name) != nil {
			return name
		}
	}
	return ""
}

func scopedPolicyFeature(scopes *yaml.Node) string {
	if scopes == nil || scopes.Kind != yaml.SequenceNode {
		return ""
	}
	for _, scope := range scopes.Content {
		if feature := mappingFeature(scope, "collectionMask"); feature != "" {
			return feature
		}
		if rules := mappingValue(scope, "rules"); rules != nil && rules.Kind == yaml.SequenceNode {
			for _, rule := range rules.Content {
				if feature := mappingFeature(rule, "fieldMask"); feature != "" {
					return feature
				}
			}
		}
		if feature := scopedPolicyFeature(mappingValue(scope, "scopes")); feature != "" {
			return feature
		}
	}
	return ""
}

func policyFromDTQLDocument(source DTQLDocument, database, reference string) (Policy, error) {
	if source.APIVersion != DTQLDocumentAPIVersion {
		return nil, fmt.Errorf("access: unsupported apiVersion %q", source.APIVersion)
	}
	visibility := source.Metadata.Visibility
	if visibility == "" {
		visibility = "public"
	}
	if visibility != "public" && visibility != "private" {
		return nil, fmt.Errorf("access: metadata.visibility must be public or private")
	}
	if source.Target.Database != database {
		return nil, fmt.Errorf("access: policy target database %q does not match configured database %q", source.Target.Database, database)
	}
	if source.Composition != "dalgo-hierarchical-v1" {
		return nil, fmt.Errorf("access: unsupported policy composition %q", source.Composition)
	}
	execution, compileErr := compileExecutionGate(source.Execution)
	if compileErr != nil {
		return nil, compileErr
	}
	var collectionMask *CompiledMask
	if source.CollectionMask != nil {
		var err error
		collectionMask, err = CompileMask(*source.CollectionMask, NameMask)
		if err != nil {
			return nil, err
		}
	}
	document := Document{APIVersion: DocumentAPIVersion, Kind: source.Kind, Metadata: DocumentMetadata{Name: source.Metadata.Name}, Default: source.Default, Bindings: source.Bindings}
	var err error
	document.Scopes, err = convertDTQLScopes(source.Scopes)
	if err != nil {
		return nil, err
	}
	if source.RuleSets != nil {
		document.RuleSets = make(map[string][]DocumentScope, len(source.RuleSets))
		for name, scopes := range source.RuleSets {
			document.RuleSets[name], err = convertDTQLScopes(scopes)
			if err != nil {
				return nil, fmt.Errorf("access: ruleSets[%s]: %w", name, err)
			}
		}
	}
	settings := decodeOptions{source: reference}
	if document.Bindings != nil || len(document.RuleSets) > 0 {
		policy, err := principalPolicySetFromDocument(document, settings)
		if err == nil {
			policy.visibility = visibility
			policy.collectionMask = collectionMask
			policy.execution = execution
		}
		return policy, err
	}
	policy, err := accessPolicyFromDocument(document, settings)
	if err == nil {
		policy.visibility = visibility
		policy.collectionMask = collectionMask
		policy.execution = execution
	}
	return policy, err
}

func convertDTQLScopes(scopes []DTQLScope) ([]DocumentScope, error) {
	result := make([]DocumentScope, len(scopes))
	for i, scope := range scopes {
		if scope.CollectionGroup != "" || scope.OpaqueQuery {
			return nil, fmt.Errorf("access: scopes[%d]: portable file policies support path scopes only", i)
		}
		if scope.CollectionMask != nil {
			return nil, fmt.Errorf("access: scopes[%d]: collectionMask is not supported by this loader", i)
		}
		rules := make([]DocumentRule, len(scope.Rules))
		for j, rule := range scope.Rules {
			rules[j] = DocumentRule{ID: rule.ID, Effect: rule.Effect, Operations: rule.Operations, Where: rule.Where, Check: rule.Check, Fields: rule.Fields, fieldMask: rule.FieldMask}
		}
		children, err := convertDTQLScopes(scope.Scopes)
		if err != nil {
			return nil, fmt.Errorf("access: scopes[%d]: %w", i, err)
		}
		result[i] = DocumentScope{Path: scope.Path, CollectionGroup: scope.CollectionGroup, OpaqueQuery: scope.OpaqueQuery, Rules: rules, Scopes: children}
	}
	return result, nil
}
