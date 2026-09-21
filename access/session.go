package access

import (
	"context"
	"errors"
	"fmt"

	"github.com/dal-go/dalgo/condeval"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

type securedReadSession struct {
	session dal.ReadSession
	guard   guard
}

func (s securedReadSession) Exists(ctx context.Context, key *record.Key) (bool, error) {
	residuals, _, err := s.guard.authorizeRequest(ctx, Request{Operation: Exists, Resources: []Resource{RecordResourceForKey(key)}})
	if err != nil {
		return false, err
	}
	if len(residuals) == 0 {
		return s.session.Exists(ctx, key)
	}
	return existsThroughRead(ctx, s.session, key, residuals[0])
}

func (s securedReadSession) Get(ctx context.Context, record record.Record) error {
	residuals, writes, err := s.guard.authorizeRequest(ctx, Request{Operation: Get, Resources: []Resource{RecordResourceForKey(record.Key())}})
	if err != nil {
		return err
	}
	if err := s.session.Get(ctx, record); err != nil {
		return err
	}
	if len(residuals) == 0 && len(writes) == 0 {
		return nil
	}
	return enforceRead(Get, record, first(residuals), first(writes))
}

func first[T any](perResource [][]T) []T {
	if len(perResource) == 0 {
		return nil
	}
	return perResource[0]
}

func (s securedReadSession) GetMulti(ctx context.Context, records []record.Record) error {
	resources := resourcesForRecords(records)
	residuals, writes, err := s.guard.authorizeRequest(ctx, Request{Operation: Get, Resources: resources})
	if err != nil {
		return err
	}
	if err := s.session.GetMulti(ctx, records); err != nil {
		return err
	}
	if len(residuals) == 0 && len(writes) == 0 {
		return nil
	}
	for i, rec := range records {
		var perRecordResiduals []residual
		if len(residuals) > 0 {
			perRecordResiduals = residuals[i]
		}
		var perRecordWrites []writeResidual
		if len(writes) > 0 {
			perRecordWrites = writes[i]
		}
		if err := enforceRead(Get, rec, perRecordResiduals, perRecordWrites); err != nil {
			for _, other := range records {
				clearRecord(other, err)
			}
			return err
		}
	}
	return nil
}

func (s securedReadSession) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	if structured, ok := query.(dal.StructuredQuery); ok && dal.HasSubquery(structured) {
		return dal.ExecuteRecursiveQuery(ctx, s, structured)
	}
	query, requested, sets, err := s.authorizeQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	if structured, ok := query.(dal.StructuredQuery); ok && sets.restrictive() {
		projection := projectQuery(structured, sets)
		if projection.status == queryProjectionEmpty {
			return nil, emptyProjectionDeniedError(query)
		}
		if projection.status == queryProjectionApplied {
			query = projection.query
		}
	}
	query = preserveRequestedQuery(query, requested)
	reader, err := s.session.ExecuteQueryToRecordsReader(ctx, query)
	if err != nil || !sets.restrictive() {
		return reader, err
	}
	return redactingReader{RecordsReader: reader, sets: sets}, nil
}

func (s securedReadSession) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	if structured, ok := query.(dal.StructuredQuery); ok && dal.HasSubquery(structured) {
		return dal.ExecuteRecursiveRecordset(ctx, s, structured, options...)
	}
	query, requested, sets, err := s.authorizeQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	if sets.restrictive() {
		structured, _ := query.(dal.StructuredQuery)
		projection := projectQuery(structured, sets)
		if projection.status != queryProjectionApplied {
			if projection.status == queryProjectionEmpty {
				return nil, emptyProjectionDeniedError(query)
			}
			return nil, &DeniedError{Decision: Decision{Operation: Query, Resource: resourcesForQuery(query)[0], Policy: "fields", Effect: effectDeny.String(), Explanation: fmt.Sprintf("the allowed fields (%s) cannot be projected onto a recordset; select explicit columns or read records", sets.sources())}}
		}
		query = projection.query
	}
	query = preserveRequestedQuery(query, requested)
	return s.session.ExecuteQueryToRecordsetReader(ctx, query, options...)
}

func emptyProjectionDeniedError(query dal.Query) error {
	return &DeniedError{Decision: Decision{Operation: Query, Resource: resourcesForQuery(query)[0], Policy: "fields", Effect: effectDeny.String(), Explanation: "no permitted columns remain after applying the query projection and field policy"}}
}

// authorizeQuery authorizes every source of a query and returns the query to
// execute — the caller's own when no residual applies, otherwise a copy whose
// Where carries the residual row condition — and the field allow-lists that
// bound its rows.
func (s securedReadSession) authorizeQuery(ctx context.Context, query dal.Query) (dal.Query, dal.StructuredQuery, fieldSets, error) {
	effective, requested := splitRequestedQuery(query)
	resources := resourcesForQuery(query)
	requestQuery := query
	if requested != nil {
		requestQuery = requested
	}
	residuals, writes, err := s.guard.authorizeRequest(ctx, Request{Operation: Query, Resources: resources, Query: requestQuery})
	if err != nil {
		return nil, nil, nil, err
	}
	rewritten, err := rewriteQuery(effective, residuals)
	if err != nil {
		return nil, nil, nil, err
	}
	for i := 1; i < len(writes); i++ {
		for _, w := range writes[i] {
			return nil, nil, nil, w.deny(Query, "", "", "field rules on a joined source are not supported in this version")
		}
	}
	sets := queryFields(first(writes))
	if requested != nil && sets.restrictive() {
		if err := validateRequestedQueryFields(requested, sets); err != nil {
			return nil, nil, nil, err
		}
	}
	return rewritten, requested, sets, nil
}

type securedWriteSession struct {
	session     dal.WriteSession
	guard       guard
	coordinator *EnforcementCoordinator
}

func (s securedWriteSession) Set(ctx context.Context, record record.Record) error {
	if s.coordinator != nil {
		op, err := protectedRecordOperation("op1", Set, record)
		if err != nil {
			return err
		}
		return s.executeProtected(ctx, []ProtectedOperation{op})
	}
	if err := s.authorizeAndEnforce(ctx, Set, []writeTarget{{key: record.Key(), data: record.Data()}}); err != nil {
		return err
	}
	return s.session.Set(ctx, record)
}

func (s securedWriteSession) SetMulti(ctx context.Context, records []record.Record) error {
	if s.coordinator != nil {
		ops, err := protectedRecordOperations(Set, records)
		if err != nil {
			return err
		}
		return s.executeProtected(ctx, ops)
	}
	if err := s.authorizeAndEnforce(ctx, Set, targetsForRecords(records)); err != nil {
		return err
	}
	return s.session.SetMulti(ctx, records)
}

func (s securedWriteSession) Insert(ctx context.Context, record record.Record, options ...dal.InsertOption) error {
	if s.coordinator != nil {
		if len(options) > 0 {
			return enforcementUnsupported(Insert, "insert options are unavailable under the protected profile")
		}
		op, err := protectedRecordOperation("op1", Insert, record)
		if err != nil {
			return err
		}
		return s.executeProtected(ctx, []ProtectedOperation{op})
	}
	if err := s.authorizeAndEnforce(ctx, Insert, []writeTarget{{key: record.Key(), data: record.Data()}}); err != nil {
		return err
	}
	return s.session.Insert(ctx, record, options...)
}

func (s securedWriteSession) InsertMulti(ctx context.Context, records []record.Record, options ...dal.InsertOption) error {
	if s.coordinator != nil {
		if len(options) > 0 {
			return enforcementUnsupported(Insert, "insert options are unavailable under the protected profile")
		}
		ops, err := protectedRecordOperations(Insert, records)
		if err != nil {
			return err
		}
		return s.executeProtected(ctx, ops)
	}
	if err := s.authorizeAndEnforce(ctx, Insert, targetsForRecords(records)); err != nil {
		return err
	}
	return s.session.InsertMulti(ctx, records, options...)
}

func (s securedWriteSession) Update(ctx context.Context, key *record.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	if s.coordinator != nil {
		if len(preconditions) > 0 {
			return enforcementUnsupported(Update, "write preconditions are unavailable under the protected profile")
		}
		op, err := NewProtectedUpdate("op1", key, updates, "")
		if err != nil {
			return err
		}
		return s.executeProtected(ctx, []ProtectedOperation{op})
	}
	if err := s.authorizeAndEnforce(ctx, Update, []writeTarget{{key: key, updates: updates}}); err != nil {
		return err
	}
	return s.session.Update(ctx, key, updates, preconditions...)
}

func (s securedWriteSession) UpdateRecord(ctx context.Context, record record.Record, updates []update.Update, preconditions ...dal.Precondition) error {
	if s.coordinator != nil {
		return s.Update(ctx, record.Key(), updates, preconditions...)
	}
	if err := s.authorizeAndEnforce(ctx, Update, []writeTarget{{key: record.Key(), updates: updates}}); err != nil {
		return err
	}
	return s.session.UpdateRecord(ctx, record, updates, preconditions...)
}

func (s securedWriteSession) UpdateMulti(ctx context.Context, keys []*record.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	if s.coordinator != nil {
		if len(preconditions) > 0 {
			return enforcementUnsupported(Update, "write preconditions are unavailable under the protected profile")
		}
		ops := make([]ProtectedOperation, len(keys))
		for i, key := range keys {
			op, err := NewProtectedUpdate(fmt.Sprintf("op%d", i+1), key, updates, "")
			if err != nil {
				return err
			}
			ops[i] = op
		}
		return s.executeProtected(ctx, ops)
	}
	targets := make([]writeTarget, len(keys))
	for i, key := range keys {
		targets[i] = writeTarget{key: key, updates: updates}
	}
	if err := s.authorizeAndEnforce(ctx, Update, targets); err != nil {
		return err
	}
	return s.session.UpdateMulti(ctx, keys, updates, preconditions...)
}

func (s securedWriteSession) Delete(ctx context.Context, key *record.Key) error {
	if s.coordinator != nil {
		op, err := NewProtectedDelete("op1", key, "")
		if err != nil {
			return err
		}
		return s.executeProtected(ctx, []ProtectedOperation{op})
	}
	if err := s.authorizeAndEnforce(ctx, Delete, []writeTarget{{key: key}}); err != nil {
		return err
	}
	return s.session.Delete(ctx, key)
}

func (s securedWriteSession) DeleteMulti(ctx context.Context, keys []*record.Key) error {
	if s.coordinator != nil {
		ops := make([]ProtectedOperation, len(keys))
		for i, key := range keys {
			op, err := NewProtectedDelete(fmt.Sprintf("op%d", i+1), key, "")
			if err != nil {
				return err
			}
			ops[i] = op
		}
		return s.executeProtected(ctx, ops)
	}
	targets := make([]writeTarget, len(keys))
	for i, key := range keys {
		targets[i] = writeTarget{key: key}
	}
	if err := s.authorizeAndEnforce(ctx, Delete, targets); err != nil {
		return err
	}
	return s.session.DeleteMulti(ctx, keys)
}

func protectedRecordOperation(id string, operation Operations, rec record.Record) (ProtectedOperation, error) {
	data, err := condeval.ToMap(rec.Data())
	if err != nil {
		return ProtectedOperation{}, fmt.Errorf("access: normalize record data: %w", err)
	}
	if operation == Insert {
		return NewProtectedInsert(id, rec.Key(), data)
	}
	return NewProtectedSet(id, rec.Key(), data, "")
}
func protectedRecordOperations(operation Operations, records []record.Record) ([]ProtectedOperation, error) {
	ops := make([]ProtectedOperation, len(records))
	for i, rec := range records {
		op, err := protectedRecordOperation(fmt.Sprintf("op%d", i+1), operation, rec)
		if err != nil {
			return nil, err
		}
		ops[i] = op
	}
	return ops, nil
}
func (s securedWriteSession) executeProtected(ctx context.Context, operations []ProtectedOperation) error {
	return s.coordinator.WithinExecution(ctx, operations, func(session ExecutionSession) error {
		assessment, err := session.Execute(ctx)
		if errors.Is(err, ErrAccessDenied) {
			decisions := make([]Decision, len(assessment.Policies))
			for i := range assessment.Policies {
				decisions[i] = cloneDecision(assessment.Policies[i].Decision)
			}
			for _, policy := range assessment.Policies {
				if !policy.Decision.Allowed {
					return &DeniedError{Decision: cloneDecision(policy.Decision), Decisions: decisions}
				}
			}
		}
		return err
	})
}

// authorizeAndEnforce authorizes a write on every target and then enforces
// the write residuals — pre-image row conditions and post-image checks —
// before anything reaches the adapter, so a batch is refused whole.
func (s securedWriteSession) authorizeAndEnforce(ctx context.Context, operation Operations, targets []writeTarget) error {
	resources := make([]Resource, len(targets))
	for i, target := range targets {
		resources[i] = RecordResourceForKey(target.key)
	}
	writes, err := s.guard.authorizeWrite(ctx, operation, resources...)
	if err != nil {
		return err
	}
	return s.enforceWrites(ctx, operation, writes, targets)
}

func targetsForRecords(records []record.Record) []writeTarget {
	targets := make([]writeTarget, len(records))
	for i, rec := range records {
		targets[i] = writeTarget{key: rec.Key(), data: rec.Data()}
	}
	return targets
}

type securedReadwriteSession struct {
	securedReadSession
	securedWriteSession
}

// SecureReadSession wraps a read session with database-bound policies.
func SecureReadSession(session dal.ReadSession, policies ...Policy) dal.ReadSession {
	return securedReadSession{session: session, guard: guard{databasePolicies: append([]Policy(nil), policies...)}}
}

// SecureWriteSession wraps a write session with database-bound policies.
func SecureWriteSession(session dal.WriteSession, policies ...Policy) dal.WriteSession {
	return securedWriteSession{session: session, guard: guard{databasePolicies: append([]Policy(nil), policies...)}}
}

// SecureReadwriteSession wraps a combined session with database-bound policies.
func SecureReadwriteSession(session dal.ReadwriteSession, policies ...Policy) dal.ReadwriteSession {
	g := guard{databasePolicies: append([]Policy(nil), policies...)}
	return securedReadwriteSession{
		securedReadSession:  securedReadSession{session: session, guard: g},
		securedWriteSession: securedWriteSession{session: session, guard: g},
	}
}

func resourcesForRecords(records []record.Record) []Resource {
	resources := make([]Resource, len(records))
	for i, record := range records {
		resources[i] = RecordResourceForKey(record.Key())
	}
	return resources
}

func resourcesForKeys(keys []*record.Key) []Resource {
	resources := make([]Resource, len(keys))
	for i, key := range keys {
		resources[i] = RecordResourceForKey(key)
	}
	return resources
}

func resourcesForQuery(query dal.Query) []Resource {
	structured, ok := query.(dal.StructuredQuery)
	if !ok {
		return []Resource{OpaqueQuery(query.String())}
	}
	from := structured.From()
	resources := []Resource{resourceForRecordsetSource(from.Base())}
	for _, join := range from.Joins() {
		resources = append(resources, resourceForRecordsetSource(join.RecordsetSource))
	}
	return resources
}

func resourceForRecordsetSource(source dal.RecordsetSource) Resource {
	switch source := source.(type) {
	case dal.CollectionRef:
		if source.Schema() != "" {
			return OpaqueQuery(source.Path())
		}
		return CollectionResourceFor(source.Parent(), source.Name())
	case *dal.CollectionRef:
		if source.Schema() != "" {
			return OpaqueQuery(source.Path())
		}
		return CollectionResourceFor(source.Parent(), source.Name())
	case dal.CollectionGroupRef:
		return CollectionGroup(source.Name())
	case *dal.CollectionGroupRef:
		return CollectionGroup(source.Name())
	default:
		return OpaqueQuery(fmt.Sprint(source))
	}
}
