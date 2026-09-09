// Package authorization defines the portable DTQL authorization result wire contract.
package authorization

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/dal-go/dalgo/access"
)

const APIVersion = "dtql.org/authorization/v1"

type Outcome string

const (
	OutcomeAllow         Outcome = "allow"
	OutcomeConditional   Outcome = "conditional"
	OutcomeDeny          Outcome = "deny"
	OutcomeIndeterminate Outcome = "indeterminate"
)

type Mode string

const (
	ModePlan      Mode = "plan"
	ModeInspect   Mode = "inspect"
	ModeSample    Mode = "sample"
	ModeExecution Mode = "execution"
)

type EvaluationCompleteness string

const (
	EvaluationComplete EvaluationCompleteness = "complete"
	EvaluationPartial  EvaluationCompleteness = "partial"
)

type DisclosureCompleteness string

const (
	DisclosureFull     DisclosureCompleteness = "full"
	DisclosureRedacted DisclosureCompleteness = "redacted"
)

type Scope string

const (
	ScopeRequest       Scope = "request"
	ScopeSample        Scope = "sample"
	ScopeDatabase      Scope = "database"
	ScopeTable         Scope = "table"
	ScopeRow           Scope = "row"
	ScopeColumn        Scope = "column"
	ScopeOperation     Scope = "operation"
	ScopePrincipal     Scope = "principal"
	ScopeConfiguration Scope = "configuration"
)

type ExecutionClass string

const (
	ExecutionDTQL            ExecutionClass = "dtql"
	ExecutionNativeSQL       ExecutionClass = "native_sql"
	ExecutionNativeGraphQL   ExecutionClass = "native_graphql"
	ExecutionStoredProcedure ExecutionClass = "stored_procedure"
)

type ReasonCode = access.ReasonCode

const (
	CodeAccessDenied           = access.CodeAccessDenied
	CodeRuleDenied             = access.CodeRuleDenied
	CodeNoMatch                = access.CodeNoMatch
	CodeRowPredicateFailed     = access.CodeRowPredicateFailed
	CodePostImageFailed        = access.CodePostImageFailed
	CodeColumnDenied           = access.CodeColumnDenied
	CodeEvaluationFailed       = access.CodeEvaluationFailed
	CodeConfigurationInvalid   = access.CodeConfigurationInvalid
	CodeSourceUnavailable      = access.CodeSourceUnavailable
	CodeEnforcementUnsupported = access.CodeEnforcementUnsupported
	CodePrincipalUnresolved    = access.CodePrincipalUnresolved
	CodeCapabilityDenied       = access.CodeCapabilityDenied
	CodeExecutionClassDenied   = access.CodeExecutionClassDenied
	CodeCallableDenied         = access.CodeCallableDenied
	CodeCollectionDenied       = access.CodeCollectionDenied
)

type Result struct {
	APIVersion   string            `json:"apiVersion"`
	RequestID    string            `json:"requestId"`
	Mode         Mode              `json:"mode"`
	Scope        Scope             `json:"scope"`
	Result       Outcome           `json:"result"`
	Allowed      bool              `json:"allowed"`
	Hypothetical bool              `json:"hypothetical"`
	Operations   []OperationResult `json:"operations"`
	Layers       []Layer           `json:"layers"`
	Blockers     []Blocker         `json:"blockers"`
	Coverage     Coverage          `json:"coverage"`
	Restrictions []Restriction     `json:"restrictions"`
	Sample       *Sample           `json:"sample,omitempty"`
}
type AuthorizationResult = Result
type OperationRef struct {
	ID                 string `json:"id"`
	RequestOperationID string `json:"requestOperationId"`
}
type OperationResult struct {
	ID                 string         `json:"id"`
	RequestOperationID string         `json:"requestOperationId"`
	Action             string         `json:"action"`
	Resource           Resource       `json:"resource"`
	Result             Outcome        `json:"result"`
	RestrictionIDs     []string       `json:"restrictionIds"`
	AllOf              []string       `json:"allOf"`
	ExecutionClass     ExecutionClass `json:"executionClass"`
	Callable           *Callable      `json:"callable,omitempty"`
}
type Resource struct {
	DatabaseID string     `json:"databaseId"`
	Path       string     `json:"path"`
	Table      string     `json:"table,omitempty"`
	RowID      string     `json:"rowId,omitempty"`
	Columns    [][]string `json:"columns,omitempty"`
}
type Callable struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}
type Source struct {
	OwnerID    string `json:"ownerId"`
	Provider   string `json:"provider"`
	DatabaseID string `json:"databaseId"`
	Kind       string `json:"kind"`
	Reference  string `json:"reference,omitempty"`
}
type PolicyRef struct {
	OwnerID    string `json:"ownerId"`
	DatabaseID string `json:"databaseId"`
	PolicyID   string `json:"policyId"`
	Revision   string `json:"revision"`
	RuleID     string `json:"ruleId,omitempty"`
}
type BindingRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type LayerDecision struct {
	OperationID    string       `json:"operationId"`
	Result         Outcome      `json:"result"`
	PolicyRef      *PolicyRef   `json:"policyRef,omitempty"`
	Scope          Scope        `json:"scope"`
	RuleRefs       []string     `json:"ruleRefs,omitempty"`
	BindingRefs    []BindingRef `json:"bindingRefs,omitempty"`
	RestrictionIDs []string     `json:"restrictionIds"`
}
type Layer struct {
	LayerID        string          `json:"layerId"`
	Source         Source          `json:"source"`
	ACLState       string          `json:"aclState"`
	Result         Outcome         `json:"result"`
	PolicyRevision string          `json:"policyRevision,omitempty"`
	Decisions      []LayerDecision `json:"decisions"`
}
type Blocker struct {
	OperationID    string         `json:"operationId"`
	Code           ReasonCode     `json:"code"`
	Scope          Scope          `json:"scope"`
	Resource       *Resource      `json:"resource,omitempty"`
	LayerID        string         `json:"layerId,omitempty"`
	PolicyRef      *PolicyRef     `json:"policyRef,omitempty"`
	Slot           string         `json:"slot,omitempty"`
	Columns        [][]string     `json:"columns,omitempty"`
	Retryable      *bool          `json:"retryable,omitempty"`
	ExecutionClass ExecutionClass `json:"executionClass,omitempty"`
	Callable       *Callable      `json:"callable,omitempty"`
}
type Restriction struct {
	ID             string                    `json:"id"`
	OperationID    string                    `json:"operationId"`
	LayerID        string                    `json:"layerId,omitempty"`
	PolicyRef      *PolicyRef                `json:"policyRef,omitempty"`
	Enforced       bool                      `json:"enforced"`
	Representation string                    `json:"representation"`
	Kind           string                    `json:"kind"`
	Expression     *access.DocumentCondition `json:"expression,omitempty"`
	Fields         []string                  `json:"fields,omitempty"`
	OmissionReason string                    `json:"omissionReason,omitempty"`
	Mask           *access.Mask              `json:"mask,omitempty"`
}
type Unevaluated struct {
	OperationID string `json:"operationId"`
	LayerID     string `json:"layerId,omitempty"`
	Reason      string `json:"reason"`
}
type Coverage struct {
	Evaluation  EvaluationCompleteness `json:"evaluation"`
	Disclosure  DisclosureCompleteness `json:"disclosure"`
	Truncated   bool                   `json:"truncated"`
	Unevaluated []Unevaluated          `json:"unevaluated"`
}
type SampleOrder struct {
	Field     []string `json:"field"`
	Direction string   `json:"direction"`
}
type Sample struct {
	RequestedLimit      int           `json:"requestedLimit"`
	EvaluatedCount      int           `json:"evaluatedCount"`
	Selection           string        `json:"selection"`
	Order               []SampleOrder `json:"order"`
	Exhaustive          bool          `json:"exhaustive"`
	TemplateOperationID string        `json:"templateOperationId"`
	SelectionResource   Resource      `json:"selectionResource"`
}

func ParseResult(data []byte) (Result, error) {
	return parseResult(data, false)
}

// DecodeResultCompatible decodes returned results while preserving unknown
// future blocker codes. All other frozen-v1 validation remains strict, and an
// unknown blocker can never coexist with an allowed result.
func DecodeResultCompatible(data []byte) (Result, error) { return parseResult(data, true) }

func parseResult(data []byte, allowUnknownReasonCodes bool) (Result, error) {
	if err := rejectDuplicateKeys(data); err != nil {
		return Result{}, fmt.Errorf("authorization result: %w", err)
	}
	if err := validateRequiredResultFields(data); err != nil {
		return Result{}, fmt.Errorf("authorization result: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("authorization result: %w", err)
	}
	if err := result.validate(allowUnknownReasonCodes); err != nil {
		return Result{}, err
	}
	return result, nil
}

func MarshalResult(result Result) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (result Result) Validate() error {
	return result.validate(false)
}

func (result Result) validate(allowUnknownReasonCodes bool) error {
	if result.APIVersion != APIVersion {
		return fmt.Errorf("authorization result: unsupported apiVersion %q", result.APIVersion)
	}
	if !validID(result.RequestID) {
		return fmt.Errorf("authorization result: requestId is required")
	}
	if !validMode(result.Mode) || !validOutcome(result.Result) || (result.Scope != ScopeRequest && result.Scope != ScopeSample) {
		return fmt.Errorf("authorization result: invalid discriminator")
	}
	if result.Allowed != (result.Result == OutcomeAllow && result.Coverage.Evaluation == EvaluationComplete) {
		return fmt.Errorf("authorization result: allowed is inconsistent with result and coverage")
	}
	if result.Allowed && (len(result.Blockers) > 0 || len(result.Restrictions) > 0) {
		return fmt.Errorf("authorization result: allowed result contains blockers or restrictions")
	}
	if result.Mode == ModeSample {
		if result.Sample == nil || result.Scope != ScopeSample {
			return fmt.Errorf("authorization result: sample mode requires sample scope and summary")
		}
	} else if result.Sample != nil || result.Scope != ScopeRequest {
		return fmt.Errorf("authorization result: non-sample mode forbids sample summary")
	}
	if len(result.Operations) > 100 || len(result.Layers) > 100 || len(result.Blockers) > 1000 || len(result.Restrictions) > 1000 {
		return fmt.Errorf("authorization result: collection limit exceeded")
	}
	if result.Operations == nil || result.Layers == nil || result.Blockers == nil || result.Restrictions == nil || result.Coverage.Unevaluated == nil {
		return fmt.Errorf("authorization result: required collections must be arrays")
	}
	if result.Coverage.Evaluation != EvaluationComplete && result.Coverage.Evaluation != EvaluationPartial {
		return fmt.Errorf("authorization result: invalid coverage evaluation")
	}
	if result.Coverage.Disclosure != DisclosureFull && result.Coverage.Disclosure != DisclosureRedacted {
		return fmt.Errorf("authorization result: invalid coverage disclosure")
	}
	restrictions := map[string]struct{}{}
	for i := range result.Restrictions {
		if err := result.Restrictions[i].validate(); err != nil {
			return fmt.Errorf("authorization result: restrictions[%d]: %w", i, err)
		}
		if _, exists := restrictions[result.Restrictions[i].ID]; exists {
			return fmt.Errorf("authorization result: duplicate restriction id %q", result.Restrictions[i].ID)
		}
		restrictions[result.Restrictions[i].ID] = struct{}{}
	}
	operations := map[string]struct{}{}
	for i := range result.Operations {
		operation := result.Operations[i]
		if operation.RestrictionIDs == nil || operation.AllOf == nil {
			return fmt.Errorf("authorization result: operation restriction collections must be arrays")
		}
		if !validID(operation.ID) || !validID(operation.RequestOperationID) || !validOutcome(operation.Result) || !validAction(operation.Action) || !validExecution(operation.ExecutionClass) || !validResource(operation.Resource) {
			return fmt.Errorf("authorization result: invalid operation %d", i)
		}
		if _, ok := operations[operation.ID]; ok {
			return fmt.Errorf("authorization result: duplicate operation id %q", operation.ID)
		}
		operations[operation.ID] = struct{}{}
		if (operation.ExecutionClass == ExecutionStoredProcedure) != (operation.Callable != nil) {
			return fmt.Errorf("authorization result: operation callable discriminator mismatch")
		}
		if !sameSet(operation.RestrictionIDs, operation.AllOf) {
			return fmt.Errorf("authorization result: operation restrictionIds and allOf differ")
		}
		for _, id := range operation.RestrictionIDs {
			if _, ok := restrictions[id]; !ok {
				return fmt.Errorf("authorization result: operation references unknown restriction %q", id)
			}
		}
	}
	for i := range result.Layers {
		if err := result.Layers[i].validate(restrictions); err != nil {
			return fmt.Errorf("authorization result: layers[%d]: %w", i, err)
		}
	}
	for i := range result.Blockers {
		if err := result.Blockers[i].validate(allowUnknownReasonCodes); err != nil {
			return fmt.Errorf("authorization result: blockers[%d]: %w", i, err)
		}
	}
	for _, u := range result.Coverage.Unevaluated {
		if !validID(u.OperationID) || !oneOf(u.Reason, "row_evidence_required", "evidence_not_authorized", "source_unavailable", "unsupported", "budget_exceeded") {
			return fmt.Errorf("authorization result: invalid unevaluated coverage")
		}
	}
	if result.Sample != nil && (result.Sample.RequestedLimit < 1 || result.Sample.RequestedLimit > 100 || result.Sample.EvaluatedCount < 0 || result.Sample.EvaluatedCount > result.Sample.RequestedLimit || result.Sample.Exhaustive || result.Sample.Selection != "readable_candidates" || !validID(result.Sample.TemplateOperationID) || !validResource(result.Sample.SelectionResource) || len(result.Sample.Order) < 1 || len(result.Sample.Order) > 32) {
		return fmt.Errorf("authorization result: invalid sample summary")
	}
	if result.Sample != nil {
		for _, order := range result.Sample.Order {
			if !validField(order.Field) || !oneOf(order.Direction, "asc", "desc") {
				return fmt.Errorf("authorization result: invalid sample order")
			}
		}
	}
	return nil
}

func (l Layer) validate(restrictions map[string]struct{}) error {
	if !bounded(l.LayerID, 256) || !oneOf(l.ACLState, "enabled", "disabled", "unavailable") || !validOutcome(l.Result) || !bounded(l.Source.OwnerID, 256) || !bounded(l.Source.Provider, 256) || !bounded(l.Source.DatabaseID, 256) || !oneOf(l.Source.Kind, "datatug", "openvaultdb", "ingitdb", "application", "adapter") {
		return fmt.Errorf("invalid layer")
	}
	if l.Decisions == nil {
		return fmt.Errorf("layer decisions must be an array")
	}
	for _, d := range l.Decisions {
		if d.RestrictionIDs == nil {
			return fmt.Errorf("decision restrictionIds must be an array")
		}
		if !validID(d.OperationID) || !validOutcome(d.Result) || !validScope(d.Scope) {
			return fmt.Errorf("invalid layer decision")
		}
		for _, id := range d.RestrictionIDs {
			if _, ok := restrictions[id]; !ok {
				return fmt.Errorf("decision references unknown restriction %q", id)
			}
		}
	}
	return nil
}
func (b Blocker) validate(allowUnknownReasonCodes bool) error {
	if !validID(b.OperationID) || (!allowUnknownReasonCodes && !validReason(b.Code)) || b.Code == "" || !validScope(b.Scope) {
		return fmt.Errorf("invalid blocker")
	}
	if b.Slot != "" && !oneOf(b.Slot, "where", "check", "fields") {
		return fmt.Errorf("invalid blocker slot")
	}
	return nil
}
func (r Restriction) validate() error {
	if !validID(r.ID) || !validID(r.OperationID) {
		return fmt.Errorf("id and operationId are required")
	}
	switch r.Representation {
	case "expression":
		if (r.Kind != "row_filter" && r.Kind != "post_image_check") || r.Expression == nil || len(r.Fields) > 0 || r.Mask != nil || r.OmissionReason != "" {
			return fmt.Errorf("invalid expression restriction")
		}
		if err := access.ValidateDocumentCondition(*r.Expression); err != nil {
			return fmt.Errorf("invalid restriction expression: %w", err)
		}
	case "fields":
		if r.Kind != "field_allowlist" || len(r.Fields) == 0 || r.Expression != nil || r.Mask != nil || r.OmissionReason != "" {
			return fmt.Errorf("invalid fields restriction")
		}
	case "reference":
		if !oneOf(r.Kind, "row_filter", "post_image_check", "field_allowlist", "opaque", "field_mask") || !oneOf(r.OmissionReason, "private", "too_large", "unsupported", "not_authorized") || r.Expression != nil || len(r.Fields) > 0 || r.Mask != nil {
			return fmt.Errorf("invalid reference restriction")
		}
	case "mask":
		if r.Kind != "field_mask" || r.Mask == nil || r.Expression != nil || len(r.Fields) > 0 || r.OmissionReason != "" {
			return fmt.Errorf("invalid mask restriction")
		}
		if _, err := access.CompileMask(*r.Mask, access.FieldMask); err != nil {
			return fmt.Errorf("invalid restriction mask: %w", err)
		}
	default:
		return fmt.Errorf("invalid restriction representation")
	}
	return nil
}

func validateRequiredResultFields(data []byte) error {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}
	require := func(object map[string]any, names ...string) error {
		for _, name := range names {
			if _, ok := object[name]; !ok {
				return fmt.Errorf("required field %q is missing", name)
			}
		}
		return nil
	}
	if err := require(root, "apiVersion", "requestId", "mode", "scope", "result", "allowed", "hypothetical", "operations", "layers", "blockers", "coverage", "restrictions"); err != nil {
		return err
	}
	objects := func(value any) []any { values, _ := value.([]any); return values }
	for _, value := range objects(root["operations"]) {
		object, _ := value.(map[string]any)
		if err := require(object, "id", "requestOperationId", "action", "resource", "result", "restrictionIds", "allOf", "executionClass"); err != nil {
			return err
		}
	}
	for _, value := range objects(root["layers"]) {
		object, _ := value.(map[string]any)
		if err := require(object, "layerId", "source", "aclState", "result", "decisions"); err != nil {
			return err
		}
		source, _ := object["source"].(map[string]any)
		if err := require(source, "ownerId", "provider", "databaseId", "kind"); err != nil {
			return err
		}
		for _, decisionValue := range objects(object["decisions"]) {
			decision, _ := decisionValue.(map[string]any)
			if err := require(decision, "operationId", "result", "scope", "restrictionIds"); err != nil {
				return err
			}
		}
	}
	for _, value := range objects(root["blockers"]) {
		object, _ := value.(map[string]any)
		if err := require(object, "operationId", "code", "scope"); err != nil {
			return err
		}
	}
	coverage, _ := root["coverage"].(map[string]any)
	if err := require(coverage, "evaluation", "disclosure", "truncated", "unevaluated"); err != nil {
		return err
	}
	for _, value := range objects(root["restrictions"]) {
		object, _ := value.(map[string]any)
		if err := require(object, "id", "operationId", "enforced", "representation", "kind"); err != nil {
			return err
		}
	}
	return nil
}
func validID(v string) bool        { return v != "" && len(v) <= 128 }
func bounded(v string, n int) bool { return v != "" && len(v) <= n }
func validOutcome(v Outcome) bool {
	return v == OutcomeAllow || v == OutcomeConditional || v == OutcomeDeny || v == OutcomeIndeterminate
}
func validMode(v Mode) bool {
	return v == ModePlan || v == ModeInspect || v == ModeSample || v == ModeExecution
}
func validField(field []string) bool {
	if len(field) < 1 || len(field) > 16 {
		return false
	}
	for _, part := range field {
		if !bounded(part, 256) {
			return false
		}
	}
	return true
}
func validResource(resource Resource) bool {
	if !bounded(resource.DatabaseID, 256) || !bounded(resource.Path, 4096) || len(resource.Columns) > 32 {
		return false
	}
	seen := map[string]struct{}{}
	for _, field := range resource.Columns {
		if !validField(field) {
			return false
		}
		key := strings.Join(field, "\x00")
		if _, ok := seen[key]; ok {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}
func validScope(v Scope) bool {
	return oneOf(string(v), "database", "table", "row", "column", "operation", "principal", "configuration")
}
func validAction(v string) bool {
	return oneOf(v, "get", "exists", "query", "insert", "set", "update", "delete", "truncate")
}
func validExecution(v ExecutionClass) bool {
	return oneOf(string(v), "dtql", "native_sql", "native_graphql", "stored_procedure")
}
func validReason(v ReasonCode) bool {
	return oneOf(string(v), "ACCESS_DENIED", "ACL_RULE_DENIED", "ACL_NO_MATCH", "ACL_ROW_PREDICATE_FAILED", "ACL_POST_IMAGE_FAILED", "ACL_COLUMN_DENIED", "ACL_EVALUATION_FAILED", "ACL_CONFIGURATION_INVALID", "ACL_SOURCE_UNAVAILABLE", "ACL_ENFORCEMENT_UNSUPPORTED", "ACL_PRINCIPAL_UNRESOLVED", "ACL_CAPABILITY_DENIED", "ACL_EXECUTION_CLASS_DENIED", "ACL_CALLABLE_DENIED", "ACL_COLLECTION_DENIED")
}
func oneOf(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
			return true
		}
	}
	return false
}
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	return reflect.DeepEqual(aa, bb)
}
func ensureEOF(d *json.Decoder) error {
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("authorization result: trailing JSON")
	}
	return nil
}
func rejectDuplicateKeys(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		token, err := d.Token()
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case json.Delim:
			switch token {
			case '{':
				seen := map[string]struct{}{}
				for d.More() {
					keyToken, err := d.Token()
					if err != nil {
						return err
					}
					key := keyToken.(string)
					if _, ok := seen[key]; ok {
						return fmt.Errorf("duplicate key %q", key)
					}
					seen[key] = struct{}{}
					if err := walk(); err != nil {
						return err
					}
				}
				_, err = d.Token()
				return err
			case '[':
				for d.More() {
					if err := walk(); err != nil {
						return err
					}
				}
				_, err = d.Token()
				return err
			}
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	return ensureEOF(d)
}
