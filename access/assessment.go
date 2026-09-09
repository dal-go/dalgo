package access

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dal-go/dalgo/dal"
)

// ReasonCode is a stable authorization result code. Hosts may safely project
// it onto the DTQL authorization wire contract without inspecting prose.
type ReasonCode string

const (
	CodeAccessDenied           ReasonCode = "ACCESS_DENIED"
	CodeRuleDenied             ReasonCode = "ACL_RULE_DENIED"
	CodeNoMatch                ReasonCode = "ACL_NO_MATCH"
	CodeRowPredicateFailed     ReasonCode = "ACL_ROW_PREDICATE_FAILED"
	CodePostImageFailed        ReasonCode = "ACL_POST_IMAGE_FAILED"
	CodeColumnDenied           ReasonCode = "ACL_COLUMN_DENIED"
	CodeEvaluationFailed       ReasonCode = "ACL_EVALUATION_FAILED"
	CodeConfigurationInvalid   ReasonCode = "ACL_CONFIGURATION_INVALID"
	CodeSourceUnavailable      ReasonCode = "ACL_SOURCE_UNAVAILABLE"
	CodeEnforcementUnsupported ReasonCode = "ACL_ENFORCEMENT_UNSUPPORTED"
	CodePrincipalUnresolved    ReasonCode = "ACL_PRINCIPAL_UNRESOLVED"
	CodeCapabilityDenied       ReasonCode = "ACL_CAPABILITY_DENIED"
	CodeExecutionClassDenied   ReasonCode = "ACL_EXECUTION_CLASS_DENIED"
	CodeCallableDenied         ReasonCode = "ACL_CALLABLE_DENIED"
	CodeCollectionDenied       ReasonCode = "ACL_COLLECTION_DENIED"
)

// IsIndeterminate reports codes that represent failed or unavailable required
// evaluation rather than a definitive policy denial.
func (code ReasonCode) IsIndeterminate() bool {
	switch code {
	case CodeEvaluationFailed, CodeConfigurationInvalid, CodeSourceUnavailable, CodeEnforcementUnsupported, CodePrincipalUnresolved:
		return true
	default:
		return false
	}
}

type DecisionScope string

const (
	DecisionScopeRequest       DecisionScope = "request"
	DecisionScopeDatabase      DecisionScope = "database"
	DecisionScopeTable         DecisionScope = "table"
	DecisionScopeRow           DecisionScope = "row"
	DecisionScopeColumn        DecisionScope = "column"
	DecisionScopeOperation     DecisionScope = "operation"
	DecisionScopePrincipal     DecisionScope = "principal"
	DecisionScopeConfiguration DecisionScope = "configuration"
)

type DecisionSlot string

const (
	DecisionSlotWhere  DecisionSlot = "where"
	DecisionSlotCheck  DecisionSlot = "check"
	DecisionSlotFields DecisionSlot = "fields"
)

type PolicyVisibility string

const (
	PolicyVisibilityPublic  PolicyVisibility = "public"
	PolicyVisibilityPrivate PolicyVisibility = "private"
)

// PolicyMetadata is trusted owner metadata. Source is an internal storage
// reference and must not be projected to an ordinary HTTP response.
type PolicyMetadata struct {
	ID         string
	Revision   string
	Visibility PolicyVisibility
	Source     string
}

type PolicyMetadataProvider interface{ PolicyMetadata() PolicyMetadata }

// DescribePolicy returns a defensive copy of a policy's trusted metadata.
// Policies without metadata are private by default.
func DescribePolicy(policy Policy) PolicyMetadata {
	if described, ok := policy.(PolicyMetadataProvider); ok {
		metadata := described.PolicyMetadata()
		if metadata.ID == "" {
			metadata.ID = policy.Name()
		}
		if metadata.Visibility == "" {
			metadata.Visibility = PolicyVisibilityPrivate
		}
		return metadata
	}
	return PolicyMetadata{ID: policy.Name(), Visibility: PolicyVisibilityPrivate}
}

type describedPolicy struct {
	Policy
	metadata PolicyMetadata
}

func (p describedPolicy) PolicyMetadata() PolicyMetadata { return p.metadata }

// WithPolicyMetadata attaches immutable owner snapshot metadata to a policy.
func WithPolicyMetadata(policy Policy, metadata PolicyMetadata) (Policy, error) {
	if policy == nil {
		return nil, fmt.Errorf("access: policy is required")
	}
	if metadata.ID == "" {
		metadata.ID = policy.Name()
	}
	if strings.TrimSpace(metadata.ID) != metadata.ID || metadata.ID == "" {
		return nil, fmt.Errorf("access: policy metadata id is invalid")
	}
	if metadata.Visibility == "" {
		metadata.Visibility = PolicyVisibilityPrivate
	}
	if metadata.Visibility != PolicyVisibilityPublic && metadata.Visibility != PolicyVisibilityPrivate {
		return nil, fmt.Errorf("access: policy metadata visibility must be public or private")
	}
	return describedPolicy{Policy: policy, metadata: metadata}, nil
}

type AssessmentOutcome string

const (
	AssessmentAllow         AssessmentOutcome = "allow"
	AssessmentConditional   AssessmentOutcome = "conditional"
	AssessmentDeny          AssessmentOutcome = "deny"
	AssessmentIndeterminate AssessmentOutcome = "indeterminate"
)

type PolicyAssessment struct {
	OperationID string
	LayerID     string
	Policy      PolicyMetadata
	Decision    Decision
}

// AssessmentRestriction is an outstanding enforceable obligation. In plan
// mode it is complete metadata, not missing evidence.
type AssessmentRestriction struct {
	OperationID   string
	PolicyIndex   int
	ResourceIndex int
	Rule          string
	Slot          DecisionSlot
	Condition     dal.Condition
	Expression    *DocumentCondition
	Write         *WriteResidual
	Fields        []string
	Opaque        bool
}

// DocumentCondition returns the portable representation of a row condition.
// A restriction whose condition is not portable returns ErrNotSerializable.
func (restriction AssessmentRestriction) DocumentCondition() (*DocumentCondition, error) {
	if restriction.Expression != nil {
		return cloneDocumentCondition(restriction.Expression), nil
	}
	if restriction.Condition == nil {
		return nil, nil
	}
	return nil, fmt.Errorf("%w: restriction has no policy-source expression", ErrNotSerializable)
}

type Assessment struct {
	Outcome      AssessmentOutcome
	Complete     bool
	Policies     []PolicyAssessment
	Restrictions []AssessmentRestriction
}

// AssessPlan evaluates policy metadata only. It performs no DAL operation and
// never reads a stored row. Callers must pass an immutable policy snapshot.
func AssessPlan(ctx context.Context, request Request, policies []Policy) Assessment {
	if len(policies) == 0 {
		return Assessment{Outcome: AssessmentIndeterminate, Complete: false, Policies: []PolicyAssessment{}, Restrictions: []AssessmentRestriction{}}
	}
	assessment := assessPolicies(ctx, request, policies)
	assessment.applyStaticFieldValidation(request)
	return assessment.public()
}

type policyAssessment struct {
	assessment         Assessment
	residuals          [][]residual
	writes             [][]writeResidual
	firstDenial        *Decision
	firstIndeterminate *Decision
}

func (a policyAssessment) public() Assessment {
	out := a.assessment
	out.Policies = append([]PolicyAssessment(nil), out.Policies...)
	out.Restrictions = append([]AssessmentRestriction(nil), out.Restrictions...)
	return out
}

func (a *policyAssessment) applyStaticFieldValidation(request Request) {
	var denials []Decision
	if query, ok := request.Query.(dal.StructuredQuery); ok && len(a.writes) > 0 {
		for _, write := range a.writes[0] {
			sets := queryFields([]writeResidual{write})
			if len(sets) == 0 {
				continue
			}
			var denied *DeniedError
			if errors.As(validateRequestedQueryFields(query, sets), &denied) {
				decision := denied.Decision
				decision.Policy, decision.PolicySource = write.policy, write.policySource
				denials = append(denials, decision)
			}
		}
	}
	for resourceIndex := range a.writes {
		for _, column := range request.Columns {
			name := strings.Join(column, ".")
			for _, write := range a.writes[resourceIndex] {
				if !writeMayAllowField(write.residual, name) {
					denials = append(denials, Decision{Operation: request.Operation, Resource: write.resource, Policy: write.policy, PolicySource: write.policySource, Effect: effectDeny.String(), Code: CodeColumnDenied, Scope: DecisionScopeColumn, Slot: DecisionSlotFields, Columns: [][]string{append([]string(nil), column...)}, Explanation: "requested field is not allowed"})
				}
			}
		}
	}
	for _, raw := range denials {
		decision := normalizeDecision(namedPolicy(raw.Policy), request, raw)
		metadata := PolicyMetadata{ID: decision.Policy, Visibility: PolicyVisibilityPrivate}
		for _, assessed := range a.assessment.Policies {
			if assessed.Decision.Policy == decision.Policy {
				metadata = assessed.Policy
				break
			}
		}
		a.assessment.Policies = append(a.assessment.Policies, PolicyAssessment{Policy: metadata, Decision: decision})
		if a.firstDenial == nil {
			copy := decision
			a.firstDenial = &copy
		}
	}
	if len(denials) > 0 {
		a.assessment.Outcome = AssessmentDeny
	}
}

type namedPolicy string

func (p namedPolicy) Name() string                           { return string(p) }
func (namedPolicy) Decide(context.Context, Request) Decision { return Decision{} }
func (namedPolicy) Authorize(context.Context, Request) error { return nil }

func writeMayAllowField(write *WriteResidual, field string) bool {
	if write == nil {
		return true
	}
	for _, alternative := range write.Alternatives {
		if alternative.fields == nil || (fieldSets{alternative.fields}).allowsWhole(field) {
			return true
		}
	}
	return write.Terminal != nil && (write.Terminal.fields == nil || (fieldSets{write.Terminal.fields}).allowsWhole(field))
}

func assessPolicies(ctx context.Context, request Request, policies []Policy) policyAssessment {
	a := policyAssessment{assessment: Assessment{Outcome: AssessmentAllow, Complete: true}}
	for policyIndex, policy := range policies {
		if policy == nil {
			decision := Decision{Operation: request.Operation, Effect: effectDeny.String(), Code: CodeEvaluationFailed, Scope: DecisionScopeConfiguration, Explanation: "nil mandatory policy"}
			a.assessment.Policies = append(a.assessment.Policies, PolicyAssessment{Decision: decision})
			a.assessment.Complete = false
			continue
		}
		decision := normalizeDecision(policy, request, policy.Decide(ctx, request))
		a.assessment.Policies = append(a.assessment.Policies, PolicyAssessment{Policy: DescribePolicy(policy), Decision: cloneDecision(decision)})
		if !decision.Allowed {
			if decision.Code.IsIndeterminate() {
				a.assessment.Complete = false
				if a.firstIndeterminate == nil {
					copy := decision
					a.firstIndeterminate = &copy
				}
			} else if a.firstDenial == nil {
				copy := decision
				a.firstDenial = &copy
			}
			continue
		}
		for i, condition := range decision.Residuals {
			if condition == nil || i >= len(request.Resources) {
				continue
			}
			if a.residuals == nil {
				a.residuals = make([][]residual, len(request.Resources))
			}
			a.residuals[i] = append(a.residuals[i], residual{policy: decision.Policy, policySource: decision.PolicySource, rule: decision.Rule, text: decision.Condition, resource: request.Resources[i], condition: condition})
			var expression *DocumentCondition
			if i < len(decision.ResidualDocuments) {
				expression = cloneDocumentCondition(decision.ResidualDocuments[i])
			}
			a.assessment.Restrictions = append(a.assessment.Restrictions, AssessmentRestriction{PolicyIndex: policyIndex, ResourceIndex: i, Rule: decision.Rule, Slot: DecisionSlotWhere, Condition: condition, Expression: expression, Opaque: expression == nil})
		}
		for i, write := range decision.Writes {
			if write == nil || i >= len(request.Resources) {
				continue
			}
			if a.writes == nil {
				a.writes = make([][]writeResidual, len(request.Resources))
			}
			a.writes[i] = append(a.writes[i], writeResidual{policy: decision.Policy, policySource: decision.PolicySource, resource: request.Resources[i], residual: write})
			a.assessment.Restrictions = append(a.assessment.Restrictions, AssessmentRestriction{PolicyIndex: policyIndex, ResourceIndex: i, Rule: decision.Rule, Slot: DecisionSlotCheck, Write: write, Opaque: true})
			fields := writeFieldRestrictions(write)
			if len(fields) > 0 {
				a.assessment.Restrictions = append(a.assessment.Restrictions, AssessmentRestriction{PolicyIndex: policyIndex, ResourceIndex: i, Rule: decision.Rule, Slot: DecisionSlotFields, Write: write, Opaque: true})
			}
		}
	}
	switch {
	case a.firstDenial != nil:
		a.assessment.Outcome = AssessmentDeny
	case !a.assessment.Complete:
		a.assessment.Outcome = AssessmentIndeterminate
	case len(a.assessment.Restrictions) > 0:
		a.assessment.Outcome = AssessmentConditional
	default:
		a.assessment.Outcome = AssessmentAllow
	}
	return a
}

func writeFieldRestrictions(write *WriteResidual) []string {
	seen := map[string]struct{}{}
	for _, alternative := range write.Alternatives {
		for _, field := range alternative.Fields {
			seen[field] = struct{}{}
		}
	}
	if write.Terminal != nil {
		for _, field := range write.Terminal.Fields {
			seen[field] = struct{}{}
		}
	}
	fields := make([]string, 0, len(seen))
	for field := range seen {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func normalizeDecision(policy Policy, request Request, decision Decision) Decision {
	if decision.Policy == "" {
		decision.Policy = policy.Name()
	}
	if decision.Operation == 0 {
		decision.Operation = request.Operation
	}
	if decision.Scope == "" {
		decision.Scope = DecisionScopeRequest
	}
	if decision.Allowed {
		if decision.Effect == "" {
			decision.Effect = effectAllow.String()
		}
		return decision
	}
	if decision.Effect != effectDeny.String() {
		decision.Allowed = false
		decision.Code = CodeEvaluationFailed
		decision.Scope = DecisionScopeConfiguration
		decision.Effect = effectDeny.String()
		decision.Explanation = "policy returned an invalid decision"
		return decision
	}
	if decision.Code == "" {
		decision.Code = CodeAccessDenied
	}
	return decision
}
