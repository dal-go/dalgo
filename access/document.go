package access

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// DocumentAPIVersion is the first stable portable policy document version.
	DocumentAPIVersion = "dalgo.io/access/v1"
	AccessPolicyKind   = "AccessPolicy"
	AuditPolicyKind    = "AuditPolicy"
)

// Document is the storage-neutral representation shared by YAML, JSON, and
// third-party codecs. YAML is the canonical human-authored encoding.
type Document struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind" yaml:"kind"`
	Metadata   DocumentMetadata `json:"metadata" yaml:"metadata"`
	Default    string           `json:"default" yaml:"default"`
	Scopes     []DocumentScope  `json:"scopes" yaml:"scopes"`
}

type DocumentMetadata struct {
	Name string `json:"name" yaml:"name"`
}

// DocumentScope selects exactly one resource kind. Path is a structural path
// fragment; nested scopes append their path to the containing scope.
type DocumentScope struct {
	Path            string          `json:"path,omitempty" yaml:"path,omitempty"`
	CollectionGroup string          `json:"collectionGroup,omitempty" yaml:"collectionGroup,omitempty"`
	OpaqueQuery     bool            `json:"opaqueQuery,omitempty" yaml:"opaqueQuery,omitempty"`
	Rules           []DocumentRule  `json:"rules,omitempty" yaml:"rules,omitempty"`
	Scopes          []DocumentScope `json:"scopes,omitempty" yaml:"scopes,omitempty"`
}

type DocumentRule struct {
	ID         string   `json:"id" yaml:"id"`
	Effect     string   `json:"effect" yaml:"effect"`
	Operations []string `json:"operations" yaml:"operations"`
}

// Codec decouples policy loading from both its syntax and its storage. A
// caller may implement this interface for HCL or another representation.
type Codec interface {
	Decode(io.Reader, *Document) error
	Encode(io.Writer, Document) error
}

type YAMLCodec struct{}

func (YAMLCodec) Decode(reader io.Reader, document *Document) error {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	if err := decoder.Decode(document); err != nil {
		return fmt.Errorf("access: decode YAML policy: %w", err)
	}
	return ensureSingleDocument(func(value any) error { return decoder.Decode(value) })
}

func (YAMLCodec) Encode(writer io.Writer, document Document) (err error) {
	encoder := yaml.NewEncoder(writer)
	encoder.SetIndent(2)
	defer func() {
		err = errors.Join(err, encoder.Close())
	}()
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("access: encode YAML policy: %w", err)
	}
	return nil
}

type JSONCodec struct{}

func (JSONCodec) Decode(reader io.Reader, document *Document) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(document); err != nil {
		return fmt.Errorf("access: decode JSON policy: %w", err)
	}
	return ensureSingleDocument(func(value any) error { return decoder.Decode(value) })
}

func (JSONCodec) Encode(writer io.Writer, document Document) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("access: encode JSON policy: %w", err)
	}
	return nil
}

func ensureSingleDocument(next func(any) error) error {
	var extra any
	err := next(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("access: a policy stream must contain exactly one document")
}

type decodeOptions struct{ source string }

type DecodeOption func(*decodeOptions)

// WithSource records where a policy document came from. It may be a file
// path, object key, URL, database key, or any application-defined reference.
func WithSource(reference string) DecodeOption {
	return func(options *decodeOptions) { options.source = strings.TrimSpace(reference) }
}

// DecodeAccessPolicy decodes and validates one AccessPolicy from any reader.
func DecodeAccessPolicy(reader io.Reader, codec Codec, options ...DecodeOption) (*AccessPolicy, error) {
	document, settings, err := decodeDocument(reader, codec, options)
	if err != nil {
		return nil, err
	}
	if document.Kind != AccessPolicyKind {
		return nil, fmt.Errorf("access: expected kind %s, got %q", AccessPolicyKind, document.Kind)
	}
	if document.Default != effectDeny.String() {
		return nil, fmt.Errorf("access: AccessPolicy default must be %q", effectDeny)
	}
	rules, err := rulesFromDocument(document, map[effect]bool{effectAllow: true, effectDeny: true})
	if err != nil {
		return nil, err
	}
	policy, err := NewPolicy(document.Metadata.Name, rules...)
	if err != nil {
		return nil, err
	}
	policy.source = settings.source
	return policy, nil
}

// DecodeAuditPolicy decodes and validates one AuditPolicy from any reader.
func DecodeAuditPolicy(reader io.Reader, codec Codec, options ...DecodeOption) (*AuditPolicy, error) {
	document, settings, err := decodeDocument(reader, codec, options)
	if err != nil {
		return nil, err
	}
	if document.Kind != AuditPolicyKind {
		return nil, fmt.Errorf("access: expected kind %s, got %q", AuditPolicyKind, document.Kind)
	}
	if document.Default != effectIgnoreAudit.String() {
		return nil, fmt.Errorf("access: AuditPolicy default must be %q", effectIgnoreAudit)
	}
	rules, err := rulesFromDocument(document, map[effect]bool{effectAudit: true, effectIgnoreAudit: true})
	if err != nil {
		return nil, err
	}
	policy, err := NewAuditPolicy(document.Metadata.Name, rules...)
	if err != nil {
		return nil, err
	}
	policy.source = settings.source
	return policy, nil
}

func decodeDocument(reader io.Reader, codec Codec, options []DecodeOption) (Document, decodeOptions, error) {
	var settings decodeOptions
	for _, option := range options {
		if option != nil {
			option(&settings)
		}
	}
	if reader == nil || codec == nil {
		return Document{}, settings, fmt.Errorf("access: policy reader and codec are required")
	}
	var document Document
	if err := codec.Decode(reader, &document); err != nil {
		return Document{}, settings, err
	}
	if document.APIVersion != DocumentAPIVersion {
		return Document{}, settings, fmt.Errorf("access: unsupported apiVersion %q", document.APIVersion)
	}
	if strings.TrimSpace(document.Metadata.Name) == "" {
		return Document{}, settings, fmt.Errorf("access: metadata.name is required")
	}
	if len(document.Scopes) == 0 {
		return Document{}, settings, fmt.Errorf("access: at least one scope is required")
	}
	return document, settings, nil
}

// EncodeAccessPolicy validates and writes one AccessPolicy through codec.
func EncodeAccessPolicy(writer io.Writer, codec Codec, policy *AccessPolicy) error {
	if policy == nil {
		return fmt.Errorf("access: policy is required")
	}
	document, err := documentFromRules(AccessPolicyKind, policy.name, effectDeny, policy.rules)
	if err != nil {
		return err
	}
	return encodeDocument(writer, codec, document)
}

// EncodeAuditPolicy validates and writes one AuditPolicy through codec.
func EncodeAuditPolicy(writer io.Writer, codec Codec, policy *AuditPolicy) error {
	if policy == nil {
		return fmt.Errorf("access: audit policy is required")
	}
	document, err := documentFromRules(AuditPolicyKind, policy.name, effectIgnoreAudit, policy.rules)
	if err != nil {
		return err
	}
	return encodeDocument(writer, codec, document)
}

func encodeDocument(writer io.Writer, codec Codec, document Document) error {
	if writer == nil || codec == nil {
		return fmt.Errorf("access: policy writer and codec are required")
	}
	return codec.Encode(writer, document)
}

func MarshalAccessPolicyYAML(policy *AccessPolicy) ([]byte, error) {
	return marshalAccessPolicy(policy, YAMLCodec{})
}

func MarshalAccessPolicyJSON(policy *AccessPolicy) ([]byte, error) {
	return marshalAccessPolicy(policy, JSONCodec{})
}

func marshalAccessPolicy(policy *AccessPolicy, codec Codec) ([]byte, error) {
	var data bytes.Buffer
	if err := EncodeAccessPolicy(&data, codec, policy); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

func MarshalAuditPolicyYAML(policy *AuditPolicy) ([]byte, error) {
	return marshalAuditPolicy(policy, YAMLCodec{})
}

func MarshalAuditPolicyJSON(policy *AuditPolicy) ([]byte, error) {
	return marshalAuditPolicy(policy, JSONCodec{})
}

func marshalAuditPolicy(policy *AuditPolicy, codec Codec) ([]byte, error) {
	var data bytes.Buffer
	if err := EncodeAuditPolicy(&data, codec, policy); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

func UnmarshalAccessPolicyYAML(data []byte, options ...DecodeOption) (*AccessPolicy, error) {
	return DecodeAccessPolicy(bytes.NewReader(data), YAMLCodec{}, options...)
}

func UnmarshalAccessPolicyJSON(data []byte, options ...DecodeOption) (*AccessPolicy, error) {
	return DecodeAccessPolicy(bytes.NewReader(data), JSONCodec{}, options...)
}

func UnmarshalAuditPolicyYAML(data []byte, options ...DecodeOption) (*AuditPolicy, error) {
	return DecodeAuditPolicy(bytes.NewReader(data), YAMLCodec{}, options...)
}

func UnmarshalAuditPolicyJSON(data []byte, options ...DecodeOption) (*AuditPolicy, error) {
	return DecodeAuditPolicy(bytes.NewReader(data), JSONCodec{}, options...)
}

func rulesFromDocument(document Document, allowedEffects map[effect]bool) ([]Rule, error) {
	rules := make([]Rule, 0, len(document.Scopes))
	for i, scope := range document.Scopes {
		rule, err := ruleFromDocumentScope(scope, allowedEffects)
		if err != nil {
			return nil, fmt.Errorf("access: scopes[%d]: %w", i, err)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func ruleFromDocumentScope(scope DocumentScope, allowedEffects map[effect]bool) (Rule, error) {
	selectors := 0
	if scope.Path != "" {
		selectors++
	}
	if scope.CollectionGroup != "" {
		selectors++
	}
	if scope.OpaqueQuery {
		selectors++
	}
	if selectors != 1 {
		return Rule{}, fmt.Errorf("a scope must select exactly one of path, collectionGroup, or opaqueQuery")
	}
	children := make([]Rule, 0, len(scope.Rules)+len(scope.Scopes))
	for i, documentRule := range scope.Rules {
		rule, err := ruleFromDocumentRule(documentRule, allowedEffects)
		if err != nil {
			return Rule{}, fmt.Errorf("rules[%d]: %w", i, err)
		}
		children = append(children, rule)
	}
	for i, childScope := range scope.Scopes {
		child, err := ruleFromDocumentScope(childScope, allowedEffects)
		if err != nil {
			return Rule{}, fmt.Errorf("scopes[%d]: %w", i, err)
		}
		children = append(children, child)
	}
	if len(children) == 0 {
		return Rule{}, fmt.Errorf("scope contains no rules")
	}
	switch {
	case scope.Path != "":
		pattern, err := parseDocumentPath(scope.Path)
		if err != nil {
			return Rule{}, err
		}
		return Under(pattern, children...), nil
	case scope.CollectionGroup != "":
		return CollectionGroupScope(scope.CollectionGroup, children...), nil
	default:
		return OpaqueQueryScope(children...), nil
	}
}

func ruleFromDocumentRule(documentRule DocumentRule, allowedEffects map[effect]bool) (Rule, error) {
	id := strings.TrimSpace(documentRule.ID)
	if id == "" {
		return Rule{}, fmt.Errorf("rule id is required")
	}
	operations, err := parseOperations(documentRule.Operations)
	if err != nil {
		return Rule{}, err
	}
	ruleEffect, err := parseEffect(documentRule.Effect)
	if err != nil {
		return Rule{}, err
	}
	if !allowedEffects[ruleEffect] {
		return Rule{}, fmt.Errorf("effect %q is not valid for policy kind %s", ruleEffect, documentRule.Effect)
	}
	return directive(ruleEffect, operations, []string{id}), nil
}

func parseEffect(value string) (effect, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case effectAllow.String():
		return effectAllow, nil
	case effectDeny.String():
		return effectDeny, nil
	case effectAudit.String():
		return effectAudit, nil
	case effectIgnoreAudit.String():
		return effectIgnoreAudit, nil
	default:
		return 0, fmt.Errorf("access: unknown effect %q", value)
	}
}

func parseDocumentPath(value string) (PathPattern, error) {
	path := strings.TrimSpace(value)
	if !strings.HasPrefix(path, "/") {
		return PathPattern{}, fmt.Errorf("path %q must start with /", value)
	}
	if path == "/" || path == "/**" {
		return PathPattern{}, nil
	}
	path = strings.TrimSuffix(path, "/**")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	patternParts := make([]any, len(parts))
	for i, part := range parts {
		if part == "" {
			return PathPattern{}, fmt.Errorf("path %q contains an empty segment", value)
		}
		decoded, err := url.PathUnescape(part)
		if err != nil {
			return PathPattern{}, fmt.Errorf("path %q has invalid escaping: %w", value, err)
		}
		if i%2 == 1 && decoded == "*" {
			patternParts[i] = AnyID
			continue
		}
		if decoded == "*" {
			return PathPattern{}, fmt.Errorf("path %q uses * where a collection name is required", value)
		}
		patternParts[i] = decoded
	}
	return NewPath(patternParts...)
}

func documentFromRules(kind, name string, defaultEffect effect, rules []Rule) (Document, error) {
	document := Document{
		APIVersion: DocumentAPIVersion,
		Kind:       kind,
		Metadata:   DocumentMetadata{Name: name},
		Default:    defaultEffect.String(),
	}
	for _, rule := range rules {
		scope, err := documentScopeFromRule(rule)
		if err != nil {
			return Document{}, err
		}
		document.Scopes = append(document.Scopes, scope)
	}
	return document, nil
}

func documentScopeFromRule(rule Rule) (DocumentScope, error) {
	var scope DocumentScope
	switch rule.kind {
	case directiveRule:
		documentRule, err := documentRuleFromRule(rule)
		if err != nil {
			return DocumentScope{}, err
		}
		scope.Path = "/"
		scope.Rules = []DocumentRule{documentRule}
		return scope, nil
	case scopeRule:
		path, err := documentPath(rule.pattern)
		if err != nil {
			return DocumentScope{}, err
		}
		scope.Path = path
	case collectionGroupRule:
		scope.CollectionGroup = rule.resource
	case opaqueQueryRule:
		scope.OpaqueQuery = true
	default:
		return DocumentScope{}, fmt.Errorf("%w: unknown rule kind", ErrNotSerializable)
	}
	for _, child := range rule.children {
		if child.kind == directiveRule {
			documentRule, err := documentRuleFromRule(child)
			if err != nil {
				return DocumentScope{}, err
			}
			scope.Rules = append(scope.Rules, documentRule)
			continue
		}
		childScope, err := documentScopeFromRule(child)
		if err != nil {
			return DocumentScope{}, err
		}
		scope.Scopes = append(scope.Scopes, childScope)
	}
	return scope, nil
}

func documentRuleFromRule(rule Rule) (DocumentRule, error) {
	if rule.kind != directiveRule || strings.TrimSpace(rule.name) == "" {
		return DocumentRule{}, fmt.Errorf("%w: every encoded directive requires an explicit rule name", ErrNotSerializable)
	}
	operations, err := operationNamesForDocument(rule.operations)
	if err != nil {
		return DocumentRule{}, fmt.Errorf("%w: %v", ErrNotSerializable, err)
	}
	return DocumentRule{ID: rule.name, Effect: rule.effect.String(), Operations: operations}, nil
}

func documentPath(pattern PathPattern) (string, error) {
	if len(pattern.segments) == 0 {
		return "/", nil
	}
	parts := make([]string, len(pattern.segments))
	for i, segment := range pattern.segments {
		if segment.anyID {
			parts[i] = "*"
			continue
		}
		value, ok := segment.value.(string)
		if !ok {
			return "", fmt.Errorf("%w: path ID %v has type %T; portable paths support string IDs", ErrNotSerializable, segment.value, segment.value)
		}
		parts[i] = url.PathEscape(value)
	}
	return "/" + strings.Join(parts, "/"), nil
}
