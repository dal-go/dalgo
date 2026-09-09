package access

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/dal-go/dalgo/condeval"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

const maxProtectedOperations = 100

var ErrDataRevisionConflict = errors.New("access: data revision conflict")
var ErrProtectedResourceUnavailable = errors.New("access: protected resource unavailable")
var ErrProtectedRecordExists = errors.New("access: protected record already exists")

// ProtectedOperation is a normalized immutable point operation passed to a
// trusted storage boundary. Construct it with the functions below.
type ProtectedOperation struct {
	id, revision string
	action       Operations
	key          *record.Key
	target       string
	data         map[string]any
	updates      []ProtectedUpdate
	columns      [][]string
}

type ProtectedUpdate struct {
	Path   []string
	Value  any
	Delete bool
}

func NewProtectedRead(id string, action Operations, key *record.Key) (ProtectedOperation, error) {
	if action != Get && action != Exists {
		return ProtectedOperation{}, fmt.Errorf("access: protected read action must be get or exists")
	}
	return newProtectedOperation(id, action, key, nil, nil, "")
}
func NewProtectedEvidenceRead(id string, action Operations, key *record.Key, fields [][]string) (ProtectedOperation, error) {
	if action != Get {
		return ProtectedOperation{}, fmt.Errorf("access: field evidence requires get authorization")
	}
	op, err := NewProtectedRead(id, action, key)
	if err != nil {
		return ProtectedOperation{}, err
	}
	if len(fields) == 0 || len(fields) > 32 {
		return ProtectedOperation{}, fmt.Errorf("access: evidence read requires between 1 and 32 fields")
	}
	for _, path := range fields {
		if len(path) == 0 {
			return ProtectedOperation{}, fmt.Errorf("access: evidence field path is empty")
		}
	}
	op.columns = clonePaths(fields)
	return op, nil
}
func NewProtectedInsert(id string, key *record.Key, data map[string]any) (ProtectedOperation, error) {
	return newProtectedOperation(id, Insert, key, data, nil, "")
}
func NewProtectedSet(id string, key *record.Key, data map[string]any, revision string) (ProtectedOperation, error) {
	return newProtectedOperation(id, Set, key, data, nil, revision)
}
func NewProtectedUpdate(id string, key *record.Key, updates []update.Update, revision string) (ProtectedOperation, error) {
	normalized := make([]ProtectedUpdate, len(updates))
	for i, item := range updates {
		if item == nil {
			return ProtectedOperation{}, fmt.Errorf("access: nil update at index %d", i)
		}
		path := append([]string(nil), item.FieldPath()...)
		if len(path) == 0 {
			path = []string{item.FieldName()}
		}
		normalized[i] = ProtectedUpdate{Path: path, Value: cloneValue(item.Value()), Delete: reflect.DeepEqual(item.Value(), update.DeleteField)}
	}
	return newProtectedOperation(id, Update, key, nil, normalized, revision)
}
func NewProtectedDelete(id string, key *record.Key, revision string) (ProtectedOperation, error) {
	return newProtectedOperation(id, Delete, key, nil, nil, revision)
}

func newProtectedOperation(id string, action Operations, key *record.Key, data map[string]any, updates []ProtectedUpdate, revision string) (ProtectedOperation, error) {
	if id == "" {
		return ProtectedOperation{}, fmt.Errorf("access: protected operation id is required")
	}
	if key == nil {
		return ProtectedOperation{}, fmt.Errorf("access: protected operation key is required")
	}
	if err := key.Validate(); err != nil {
		return ProtectedOperation{}, fmt.Errorf("access: protected operation key: %w", err)
	}
	return ProtectedOperation{id: id, action: action, key: cloneKey(key), target: key.String(), data: cloneMap(data), updates: cloneProtectedUpdates(updates), revision: revision}, nil
}

func (o ProtectedOperation) ID() string                 { return o.id }
func (o ProtectedOperation) Action() Operations         { return o.action }
func (o ProtectedOperation) Key() *record.Key           { return cloneKey(o.key) }
func (o ProtectedOperation) CanonicalTarget() string    { return o.target }
func (o ProtectedOperation) Data() map[string]any       { return cloneMap(o.data) }
func (o ProtectedOperation) Updates() []ProtectedUpdate { return cloneProtectedUpdates(o.updates) }
func (o ProtectedOperation) IfDataRevision() string     { return o.revision }
func (o ProtectedOperation) Columns() [][]string        { return clonePaths(o.columns) }

func cloneProtectedUpdates(in []ProtectedUpdate) []ProtectedUpdate {
	out := make([]ProtectedUpdate, len(in))
	for i := range in {
		out[i] = ProtectedUpdate{Path: append([]string(nil), in[i].Path...), Value: cloneValue(in[i].Value), Delete: in[i].Delete}
	}
	return out
}
func cloneValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return condeval.CloneMap(value)
	case []any:
		out := make([]any, len(value))
		for i := range value {
			out[i] = cloneValue(value[i])
		}
		return out
	}
	return value
}
func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	return condeval.CloneMap(value)
}
func cloneKey(key *record.Key) *record.Key {
	if key == nil {
		return nil
	}
	var parent *record.Key
	if key.Parent() != nil {
		parent = cloneKey(key.Parent())
	}
	options := []record.KeyOption{record.WithKeyID(key.ID)}
	if parent != nil {
		options = append(options, record.WithParentKey(parent))
	}
	clone, err := record.NewKeyWithOptions(key.Collection(), options...)
	if err != nil {
		panic(err)
	}
	return clone
}

// ProtectedEvidence is private storage evidence supplied only inside the
// trusted storage callback. PreparationError represents a row-dependent
// failure to construct complete evidence. Its cause remains private and the
// item must carry no images, revisions, or existence fact. ProtectedEvidence
// must never be copied into Assessment.
type ProtectedEvidence struct {
	OperationID, CanonicalTarget, SnapshotToken, DataRevision, CandidateRevision string
	Exists, Complete                                                             bool
	PreImage, CandidateImage                                                     map[string]any
	PreparationError                                                             error
}

type ProtectedInspectionStorage interface {
	Evidence(context.Context) ([]ProtectedEvidence, error)
}
type ProtectedExecutionStorage interface {
	ProtectedInspectionStorage
	Execute(context.Context) error
}
type ProtectedStorage interface {
	WithinProtectedInspection(context.Context, []ProtectedOperation, func(ProtectedInspectionStorage) error) error
	WithinProtectedExecution(context.Context, []ProtectedOperation, func(ProtectedExecutionStorage) error) error
}

type PolicyLease interface {
	Policies() []Policy
	Revision() string
	Release()
}
type PolicyLeaseProvider func(context.Context) (PolicyLease, error)
type MandatoryParticipant struct {
	LayerID   string
	Provider  PolicyLeaseProvider
	Validator CandidateValidator
}
type staticPolicyLease struct{ policies []Policy }

func (l *staticPolicyLease) Policies() []Policy { return append([]Policy(nil), l.policies...) }
func (*staticPolicyLease) Revision() string     { return "static" }
func (*staticPolicyLease) Release()             {}
func NewStaticPolicyLease(policies ...Policy) (PolicyLease, error) {
	if len(policies) == 0 {
		return nil, fmt.Errorf("access: static policy lease requires policies")
	}
	for i, p := range policies {
		if p == nil {
			return nil, fmt.Errorf("access: nil static policy at index %d", i)
		}
	}
	return &staticPolicyLease{policies: append([]Policy(nil), policies...)}, nil
}
func NewStaticParticipant(layerID string, policies ...Policy) (MandatoryParticipant, error) {
	lease, err := NewStaticPolicyLease(policies...)
	if err != nil {
		return MandatoryParticipant{}, err
	}
	if layerID == "" {
		return MandatoryParticipant{}, fmt.Errorf("access: layer id is required")
	}
	return MandatoryParticipant{LayerID: layerID, Provider: func(context.Context) (PolicyLease, error) { return lease, nil }}, nil
}

type InspectionSession interface {
	Assess(context.Context) (Assessment, error)
	Evidence(context.Context) ([]AuthorizedPointEvidence, error)
	ReadVisibility(context.Context) (map[string]bool, error)
	ReadVisibilityFor(context.Context, Principal) (map[string]bool, error)
	isInspectionSession()
}
type AuthorizedFieldEvidence struct {
	Path    []string
	Present bool
	Value   any
}
type AuthorizedPointEvidence struct {
	OperationID  string
	Exists       bool
	DataRevision string
	Fields       []AuthorizedFieldEvidence
}
type ExecutionSession interface {
	InspectionSession
	Execute(context.Context) (Assessment, error)
	Receipts(context.Context) ([]ExecutionReceipt, error)
	Revisions(context.Context) (map[string]string, error)
	executionSession()
}
type ExecutionReceipt struct {
	OperationID  string
	DataRevision string
}

type EnforcementCoordinator struct {
	storage                ProtectedStorage
	participants           []MandatoryParticipant
	validationParticipants []MandatoryParticipant
	validator              CandidateValidator
}

// CandidateValidator performs pure schema/business validation of one complete
// final image. It runs inside the pinned storage boundary before ACL
// evaluation. Implementations must not mutate the image or cause effects.
type CandidateValidator func(context.Context, ProtectedOperation, map[string]any) error

func NewEnforcementCoordinator(storage ProtectedStorage, participants ...MandatoryParticipant) (*EnforcementCoordinator, error) {
	return NewValidatedEnforcementCoordinator(storage, nil, participants...)
}
func NewValidatedEnforcementCoordinator(storage ProtectedStorage, validator CandidateValidator, participants ...MandatoryParticipant) (*EnforcementCoordinator, error) {
	if storage == nil {
		return nil, fmt.Errorf("access: protected storage is required")
	}
	if len(participants) == 0 {
		return nil, fmt.Errorf("access: at least one mandatory participant is required")
	}
	seen := map[string]bool{}
	var policyParticipants, validationParticipants []MandatoryParticipant
	for i, p := range participants {
		if p.LayerID == "" || (p.Provider == nil && p.Validator == nil) || seen[p.LayerID] {
			return nil, fmt.Errorf("access: invalid mandatory participant at index %d", i)
		}
		seen[p.LayerID] = true
		if p.Provider != nil {
			policyParticipants = append(policyParticipants, p)
		}
		if p.Validator != nil {
			validationParticipants = append(validationParticipants, p)
		}
	}
	if len(policyParticipants) == 0 {
		return nil, fmt.Errorf("access: at least one mandatory policy participant is required")
	}
	return &EnforcementCoordinator{storage: storage, participants: append([]MandatoryParticipant(nil), policyParticipants...), validationParticipants: append([]MandatoryParticipant(nil), validationParticipants...), validator: validator}, nil
}

func (c *EnforcementCoordinator) WithinInspection(ctx context.Context, operations []ProtectedOperation, inspect func(InspectionSession) error) error {
	operations = cloneProtectedOperations(operations)
	if inspect == nil {
		return fmt.Errorf("access: inspection callback is required")
	}
	if err := validateProtectedOperations(operations, false); err != nil {
		return err
	}
	var leases []PolicyLease
	defer func() { releaseLeases(leases) }()
	err := c.storage.WithinProtectedInspection(ctx, cloneProtectedOperations(operations), func(storage ProtectedInspectionStorage) error {
		var err error
		leases, err = c.acquire(ctx)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		evidence, err := storage.Evidence(ctx)
		if err != nil {
			return err
		}
		validationErr := c.validateCandidates(ctx, operations, evidence)
		session, err := newInspectionSession(ctx, operations, evidence, c.participants, leases, validationErr, true)
		if err != nil {
			return err
		}
		defer session.close()
		return inspect(session)
	})
	return err
}

func (c *EnforcementCoordinator) WithinExecution(ctx context.Context, operations []ProtectedOperation, execute func(ExecutionSession) error) error {
	operations = cloneProtectedOperations(operations)
	if execute == nil {
		return fmt.Errorf("access: execution callback is required")
	}
	if err := validateProtectedOperations(operations, true); err != nil {
		return err
	}
	var leases []PolicyLease
	defer func() { releaseLeases(leases) }()
	err := c.storage.WithinProtectedExecution(ctx, cloneProtectedOperations(operations), func(storage ProtectedExecutionStorage) error {
		var err error
		leases, err = c.acquire(ctx)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		evidence, err := storage.Evidence(ctx)
		if err != nil {
			return err
		}
		validationErr := c.validateCandidates(ctx, operations, evidence)
		session, err := newExecutionSession(ctx, operations, evidence, c.participants, leases, storage, validationErr)
		if err != nil {
			return err
		}
		defer session.close()
		if err := execute(session); err != nil {
			return err
		}
		if !session.succeeded {
			return fmt.Errorf("access: execution callback returned without successful Execute")
		}
		return nil
	})
	return err
}
func (c *EnforcementCoordinator) validateCandidates(ctx context.Context, operations []ProtectedOperation, evidence []ProtectedEvidence) error {
	type registeredValidator struct {
		layer    string
		validate CandidateValidator
	}
	validators := make([]registeredValidator, 0, len(c.validationParticipants)+1)
	if c.validator != nil {
		validators = append(validators, registeredValidator{validate: c.validator})
	}
	for _, participant := range c.validationParticipants {
		if participant.Validator != nil {
			validators = append(validators, registeredValidator{layer: participant.LayerID, validate: participant.Validator})
		}
	}
	byID := map[string]ProtectedEvidence{}
	for _, item := range evidence {
		byID[item.OperationID] = item
	}
	for _, op := range operations {
		if op.action == Get || op.action == Exists {
			continue
		}
		item, ok := byID[op.id]
		if ok && item.PreparationError != nil {
			continue
		}
		if !ok || (op.action != Delete && item.CandidateImage == nil) || (op.action == Delete && item.CandidateImage != nil) {
			return fmt.Errorf("access: candidate validation lacks complete image for %q", op.id)
		}
		if item.CandidateRevision == "" {
			return fmt.Errorf("access: candidate validation lacks revision for %q", op.id)
		}
		if op.action == Delete {
			continue
		}
		for _, validator := range validators {
			if err := validator.validate(ctx, op, condeval.CloneMap(item.CandidateImage)); err != nil {
				return fmt.Errorf("access: candidate validation for %q at layer %q: %w", op.id, validator.layer, err)
			}
		}
	}
	return nil
}

func (c *EnforcementCoordinator) acquire(ctx context.Context) ([]PolicyLease, error) {
	leases := make([]PolicyLease, 0, len(c.participants))
	for _, p := range c.participants {
		lease, err := p.Provider(ctx)
		if ctxErr := ctx.Err(); ctxErr != nil {
			if lease != nil {
				lease.Release()
			}
			releaseLeases(leases)
			return nil, ctxErr
		}
		var policies []Policy
		if lease != nil {
			policies = lease.Policies()
		}
		if err != nil || lease == nil || len(policies) == 0 || len(policies) > 100 {
			if lease != nil {
				lease.Release()
			}
			code := CodeSourceUnavailable
			if err == nil {
				err = fmt.Errorf("invalid policy snapshot")
				code = CodeConfigurationInvalid
			}
			leases = append(leases, &unavailablePolicyLease{policy: unavailablePolicy{name: p.LayerID, code: code, cause: err}})
			continue
		}
		leases = append(leases, &pinnedPolicyLease{PolicyLease: lease, policies: append([]Policy(nil), policies...)})
	}
	return leases, nil
}

type unavailablePolicy struct {
	name  string
	code  ReasonCode
	cause error
}

func (p unavailablePolicy) Name() string       { return p.name }
func (unavailablePolicy) InspectionPure() bool { return true }
func (p unavailablePolicy) PolicyMetadata() PolicyMetadata {
	return PolicyMetadata{ID: p.name, Visibility: PolicyVisibilityPrivate}
}
func (p unavailablePolicy) Decide(_ context.Context, request Request) Decision {
	decision := Decision{Operation: request.Operation, Policy: p.name, Effect: effectDeny.String(), Code: p.code, Scope: DecisionScopeConfiguration, Explanation: "mandatory policy participant is unavailable"}
	if len(request.Resources) > 0 {
		decision.Resource = request.Resources[0]
	}
	return decision
}
func (p unavailablePolicy) Authorize(ctx context.Context, request Request) error {
	return &DeniedError{Decision: p.Decide(ctx, request)}
}

type unavailablePolicyLease struct{ policy unavailablePolicy }

func (l *unavailablePolicyLease) Policies() []Policy { return []Policy{l.policy} }
func (*unavailablePolicyLease) Revision() string     { return "" }
func (*unavailablePolicyLease) Release()             {}

type pinnedPolicyLease struct {
	PolicyLease
	policies []Policy
}

func (l *pinnedPolicyLease) Policies() []Policy { return append([]Policy(nil), l.policies...) }
func releaseLeases(leases []PolicyLease) {
	for i := len(leases) - 1; i >= 0; i-- {
		leases[i].Release()
	}
}

func validateProtectedOperations(ops []ProtectedOperation, unique bool) error {
	if len(ops) == 0 || len(ops) > maxProtectedOperations {
		return fmt.Errorf("access: protected operation count must be between 1 and %d", maxProtectedOperations)
	}
	ids, targets := map[string]bool{}, map[string]bool{}
	for _, op := range ops {
		if op.id == "" || ids[op.id] {
			return fmt.Errorf("access: duplicate or empty operation id %q", op.id)
		}
		ids[op.id] = true
		if unique && targets[op.target] {
			return fmt.Errorf("access: duplicate record target %q", op.target)
		}
		targets[op.target] = true
	}
	return nil
}

type inspectionSession struct {
	mu               sync.Mutex
	alive            bool
	assessment       Assessment
	operations       []ProtectedOperation
	evidence         []ProtectedEvidence
	revisionConflict bool
	admissionErr     error
	ingress          context.Context
	participants     []MandatoryParticipant
	leases           []PolicyLease
}

func (s *inspectionSession) ReadVisibility(ctx context.Context) (map[string]bool, error) {
	return s.readVisibility(ctx, nil)
}
func (s *inspectionSession) ReadVisibilityFor(ctx context.Context, requester Principal) (map[string]bool, error) {
	if requester.Subject != nil {
		if err := requester.Subject.Validate(); err != nil {
			return nil, err
		}
	}
	if requester.Actor != nil {
		if err := requester.Actor.Validate(); err != nil {
			return nil, err
		}
	}
	return s.readVisibility(ctx, &requester)
}
func (s *inspectionSession) readVisibility(ctx context.Context, requester *Principal) (map[string]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.alive {
		return nil, fmt.Errorf("access: inspection session is closed")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.ingress.Err(); err != nil {
		return nil, err
	}
	target, err := s.evaluateReadVisibility(s.ingress)
	if err != nil {
		return nil, err
	}
	if requester == nil {
		return target, nil
	}
	requesterCtx := WithPrincipal(ctx, *requester)
	other, err := s.evaluateReadVisibility(requesterCtx)
	if err != nil {
		return nil, err
	}
	for id, visible := range target {
		target[id] = visible && other[id]
	}
	return target, nil
}
func (s *inspectionSession) evaluateReadVisibility(ctx context.Context) (map[string]bool, error) {
	result := make(map[string]bool, len(s.operations))
	for _, op := range s.operations {
		read := op
		read.action = Get
		read.revision = ""
		read.columns = nil
		assessment, err := assessProtected(ctx, []ProtectedOperation{read}, filterEvidence(s.evidence, op.id), s.participants, s.leases, true)
		if err != nil {
			return nil, err
		}
		result[op.id] = assessment.Outcome == AssessmentAllow && evidenceExists(s.evidence, op.id)
	}
	return result, nil
}

func (s *inspectionSession) Evidence(ctx context.Context) ([]AuthorizedPointEvidence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.alive {
		return nil, fmt.Errorf("access: inspection session is closed")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.ingress.Err(); err != nil {
		return nil, err
	}
	if s.assessment.Outcome != AssessmentAllow {
		return nil, ErrAccessDenied
	}
	if s.admissionErr != nil {
		return nil, s.admissionErr
	}
	if s.revisionConflict {
		return nil, ErrDataRevisionConflict
	}
	result := make([]AuthorizedPointEvidence, 0, len(s.operations))
	for _, op := range s.operations {
		if len(op.columns) == 0 {
			continue
		}
		var item ProtectedEvidence
		for _, candidate := range s.evidence {
			if candidate.OperationID == op.id {
				item = candidate
				break
			}
		}
		point := AuthorizedPointEvidence{OperationID: op.id, Exists: item.Exists, DataRevision: item.DataRevision}
		for _, path := range op.columns {
			value, present := valueAtPath(item.PreImage, path)
			point.Fields = append(point.Fields, AuthorizedFieldEvidence{Path: append([]string(nil), path...), Present: present, Value: cloneValue(value)})
		}
		result = append(result, point)
	}
	return result, nil
}

func (*inspectionSession) isInspectionSession() {}
func (s *inspectionSession) Assess(ctx context.Context) (Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.alive {
		return Assessment{}, fmt.Errorf("access: inspection session is closed")
	}
	if err := ctx.Err(); err != nil {
		return Assessment{}, err
	}
	if err := s.ingress.Err(); err != nil {
		return Assessment{}, err
	}
	assessment := cloneAssessment(s.assessment)
	if assessment.Outcome == AssessmentAllow && s.admissionErr != nil {
		return assessment, s.admissionErr
	}
	if s.revisionConflict && assessment.Outcome == AssessmentAllow {
		return assessment, ErrDataRevisionConflict
	}
	return assessment, nil
}
func (s *inspectionSession) close() { s.mu.Lock(); s.alive = false; s.mu.Unlock() }

type executionSession struct {
	*inspectionSession
	storage   ProtectedExecutionStorage
	executed  bool
	succeeded bool
	receipts  []ExecutionReceipt
}

func (*executionSession) executionSession() {}
func (s *executionSession) Execute(ctx context.Context) (Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.alive {
		return Assessment{}, fmt.Errorf("access: execution session is closed")
	}
	if err := s.ingress.Err(); err != nil {
		return Assessment{}, err
	}
	if err := ctx.Err(); err != nil {
		return Assessment{}, err
	}
	if s.executed {
		return Assessment{}, fmt.Errorf("access: execution session already used")
	}
	s.executed = true
	if s.assessment.Outcome != AssessmentAllow {
		return cloneAssessment(s.assessment), ErrAccessDenied
	}
	if s.admissionErr != nil {
		return cloneAssessment(s.assessment), s.admissionErr
	}
	if s.revisionConflict {
		return cloneAssessment(s.assessment), ErrDataRevisionConflict
	}
	if err := s.storage.Execute(ctx); err != nil {
		return cloneAssessment(s.assessment), err
	}
	s.succeeded = true
	for _, item := range s.evidence {
		s.receipts = append(s.receipts, ExecutionReceipt{OperationID: item.OperationID, DataRevision: item.CandidateRevision})
	}
	return cloneAssessment(s.assessment), nil
}
func (s *executionSession) Receipts(ctx context.Context) ([]ExecutionReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.alive {
		return nil, fmt.Errorf("access: execution session is closed")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.ingress.Err(); err != nil {
		return nil, err
	}
	if !s.succeeded {
		return nil, fmt.Errorf("access: execution receipts are unavailable before successful Execute")
	}
	return append([]ExecutionReceipt(nil), s.receipts...), nil
}
func (s *executionSession) Revisions(ctx context.Context) (map[string]string, error) {
	receipts, err := s.Receipts(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(receipts))
	for _, receipt := range receipts {
		result[receipt.OperationID] = receipt.DataRevision
	}
	return result, nil
}

func newInspectionSession(ctx context.Context, ops []ProtectedOperation, evidence []ProtectedEvidence, participants []MandatoryParticipant, leases []PolicyLease, validationErr error, inspection bool) (*inspectionSession, error) {
	assessment, err := assessProtected(ctx, ops, evidence, participants, leases, inspection)
	if err != nil {
		return nil, err
	}
	conflict := false
	admissionErr := validationErr
	for _, op := range ops {
		for _, item := range evidence {
			if item.OperationID != op.id {
				continue
			}
			if item.PreparationError != nil {
				continue
			}
			if op.revision != "" && item.DataRevision != op.revision {
				conflict = true
			}
			if (op.action == Get || op.action == Exists || op.action == Update || op.action == Delete) && !item.Exists {
				admissionErr = ErrProtectedResourceUnavailable
			}
			if op.action == Insert && item.Exists {
				admissionErr = ErrProtectedRecordExists
			}
		}
	}
	return &inspectionSession{alive: true, assessment: assessment, operations: append([]ProtectedOperation(nil), ops...), evidence: cloneProtectedEvidence(evidence), revisionConflict: conflict, admissionErr: admissionErr, ingress: ctx, participants: append([]MandatoryParticipant(nil), participants...), leases: append([]PolicyLease(nil), leases...)}, nil
}
func newExecutionSession(ctx context.Context, ops []ProtectedOperation, evidence []ProtectedEvidence, participants []MandatoryParticipant, leases []PolicyLease, storage ProtectedExecutionStorage, validationErr error) (*executionSession, error) {
	inspection, err := newInspectionSession(ctx, ops, evidence, participants, leases, validationErr, false)
	if err != nil {
		return nil, err
	}
	return &executionSession{inspectionSession: inspection, storage: storage}, nil
}

func assessProtected(ctx context.Context, ops []ProtectedOperation, evidence []ProtectedEvidence, participants []MandatoryParticipant, leases []PolicyLease, inspection bool) (Assessment, error) {
	if len(evidence) != len(ops) {
		return Assessment{}, fmt.Errorf("access: incomplete protected evidence")
	}
	byID := map[string]ProtectedEvidence{}
	for _, item := range evidence {
		if byID[item.OperationID].OperationID != "" || item.OperationID == "" {
			return Assessment{}, fmt.Errorf("access: invalid protected evidence")
		}
		failed := item.PreparationError != nil
		if failed {
			if item.Complete || item.SnapshotToken != "" || item.DataRevision != "" || item.CandidateRevision != "" || item.Exists || item.PreImage != nil || item.CandidateImage != nil {
				return Assessment{}, fmt.Errorf("access: invalid failed protected evidence")
			}
		} else if !item.Complete || item.SnapshotToken == "" {
			return Assessment{}, fmt.Errorf("access: invalid protected evidence")
		}
		byID[item.OperationID] = item
	}
	aggregate := Assessment{Outcome: AssessmentAllow, Complete: true}
	for _, op := range ops {
		item, ok := byID[op.id]
		if !ok || item.CanonicalTarget != op.target {
			return Assessment{}, fmt.Errorf("access: protected evidence does not match operation %q", op.id)
		}
		if item.Exists && item.PreImage == nil {
			return Assessment{}, fmt.Errorf("access: present evidence lacks complete pre-image for %q", op.id)
		}
		if !item.Exists && item.PreImage != nil {
			return Assessment{}, fmt.Errorf("access: absent evidence carries a pre-image for %q", op.id)
		}
		request := Request{Operation: op.action, Resources: []Resource{RecordResourceForKey(op.key)}, Columns: clonePaths(op.columns)}
		for i, lease := range leases {
			internal := assessPoliciesMode(ctx, request, lease.Policies(), inspection)
			internal.applyStaticFieldValidation(request)
			planned := internal.public()
			for j := range planned.Policies {
				planned.Policies[j].LayerID = participants[i].LayerID
				planned.Policies[j].OperationID = op.id
			}
			evaluated := planned
			if item.PreparationError == nil {
				evaluated = evaluateEvidence(op, item, planned)
			}
			for j := range evaluated.Restrictions {
				evaluated.Restrictions[j].OperationID = op.id
			}
			aggregate.Policies = append(aggregate.Policies, evaluated.Policies...)
			aggregate.Restrictions = append(aggregate.Restrictions, evaluated.Restrictions...)
			aggregate.Outcome = reduceOutcome(aggregate.Outcome, evaluated.Outcome)
			aggregate.Complete = aggregate.Complete && evaluated.Complete
		}
		if item.PreparationError != nil {
			aggregate.Policies = append(aggregate.Policies, PolicyAssessment{
				OperationID: op.id,
				Policy:      PolicyMetadata{ID: "protected-evidence", Visibility: PolicyVisibilityPrivate},
				Decision: Decision{
					Operation:   op.action,
					Resource:    RecordResourceForKey(op.key),
					Policy:      "protected-evidence",
					Effect:      effectDeny.String(),
					Code:        CodeEvaluationFailed,
					Scope:       DecisionScopeOperation,
					Explanation: "complete protected evidence could not be prepared",
				},
			})
			aggregate.Outcome = reduceOutcome(aggregate.Outcome, AssessmentIndeterminate)
			aggregate.Complete = false
		}
	}
	return aggregate, nil
}

func evaluateEvidence(op ProtectedOperation, evidence ProtectedEvidence, assessment Assessment) Assessment {
	for i := range assessment.Policies {
		d := &assessment.Policies[i].Decision
		if !d.Allowed {
			continue
		}
		var data map[string]any
		if op.action == Insert {
			data = evidence.CandidateImage
		} else {
			data = evidence.PreImage
		}
		for _, c := range d.Residuals {
			ok, err := condeval.Match(data, c)
			if err != nil {
				d.Allowed = false
				d.Code = CodeEvaluationFailed
			} else if !ok {
				d.Allowed = false
				d.Code = CodeRowPredicateFailed
				d.Scope = DecisionScopeRow
				d.Slot = DecisionSlotWhere
			}
		}
		if d.Allowed && len(d.Writes) > 0 && d.Writes[0] != nil && (op.action == Insert || op.action == Set || op.action == Update || op.action == Delete) {
			images := writeImages{exists: evidence.Exists, pre: condeval.CloneMap(evidence.PreImage), post: condeval.CloneMap(evidence.CandidateImage), updates: operationUpdates(op)}
			w := writeResidual{policy: d.Policy, policySource: d.PolicySource, resource: d.Resource, residual: d.Writes[0]}
			if err := evaluateWrite(op.action, images, w); err != nil {
				d.Allowed = false
				var denied *DeniedError
				if errors.As(err, &denied) {
					d.Rule, d.Condition, d.Explanation = denied.Decision.Rule, denied.Decision.Condition, denied.Decision.Explanation
					d.Code, d.Scope, d.Slot, d.Columns = denied.Decision.Code, denied.Decision.Scope, denied.Decision.Slot, clonePaths(denied.Decision.Columns)
				}
				d.Effect = effectDeny.String()
			}
		}
		if !d.Allowed {
			if d.Code.IsIndeterminate() {
				assessment.Complete = false
				assessment.Outcome = reduceOutcome(assessment.Outcome, AssessmentIndeterminate)
			} else {
				assessment.Outcome = AssessmentDeny
			}
		}
	}
	if assessment.Outcome == AssessmentConditional {
		assessment.Outcome = AssessmentAllow
		assessment.Restrictions = nil
	}
	return assessment
}

func operationUpdates(op ProtectedOperation) []update.Update {
	result := make([]update.Update, len(op.updates))
	for i, item := range op.updates {
		if item.Delete {
			result[i] = update.DeleteByFieldPath(item.Path...)
			continue
		}
		result[i] = update.ByFieldPath(append(update.FieldPath(nil), item.Path...), cloneValue(item.Value))
	}
	return result
}
func reduceOutcome(a, b AssessmentOutcome) AssessmentOutcome {
	rank := map[AssessmentOutcome]int{AssessmentAllow: 0, AssessmentConditional: 1, AssessmentIndeterminate: 2, AssessmentDeny: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
func cloneAssessment(a Assessment) Assessment {
	a.Policies = append([]PolicyAssessment(nil), a.Policies...)
	for i := range a.Policies {
		a.Policies[i].Decision = cloneDecision(a.Policies[i].Decision)
	}
	a.Restrictions = append([]AssessmentRestriction(nil), a.Restrictions...)
	return a
}
func cloneProtectedEvidence(in []ProtectedEvidence) []ProtectedEvidence {
	out := make([]ProtectedEvidence, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].PreImage = cloneMap(in[i].PreImage)
		out[i].CandidateImage = cloneMap(in[i].CandidateImage)
	}
	return out
}
func cloneProtectedOperations(in []ProtectedOperation) []ProtectedOperation {
	out := make([]ProtectedOperation, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].key = cloneKey(in[i].key)
		out[i].data = cloneMap(in[i].data)
		out[i].updates = cloneProtectedUpdates(in[i].updates)
		out[i].columns = clonePaths(in[i].columns)
	}
	return out
}
func valueAtPath(data map[string]any, path []string) (any, bool) {
	var current any = data
	for _, part := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}
func filterEvidence(items []ProtectedEvidence, id string) []ProtectedEvidence {
	for _, item := range items {
		if item.OperationID == id {
			return []ProtectedEvidence{item}
		}
	}
	return nil
}
func evidenceExists(items []ProtectedEvidence, id string) bool {
	for _, item := range items {
		if item.OperationID == id {
			return item.Exists
		}
	}
	return false
}
