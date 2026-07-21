package access

import (
	"context"
	"fmt"

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
	if err := s.guard.authorize(ctx, Exists, RecordResourceForKey(key)); err != nil {
		return false, err
	}
	return s.session.Exists(ctx, key)
}

func (s securedReadSession) Get(ctx context.Context, record record.Record) error {
	if err := s.guard.authorize(ctx, Get, RecordResourceForKey(record.Key())); err != nil {
		return err
	}
	return s.session.Get(ctx, record)
}

func (s securedReadSession) GetMulti(ctx context.Context, records []record.Record) error {
	resources := resourcesForRecords(records)
	if err := s.guard.authorize(ctx, Get, resources...); err != nil {
		return err
	}
	return s.session.GetMulti(ctx, records)
}

func (s securedReadSession) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	resources := resourcesForQuery(query)
	if err := s.guard.authorizeRequest(ctx, Request{Operation: Query, Resources: resources, Query: query}); err != nil {
		return nil, err
	}
	return s.session.ExecuteQueryToRecordsReader(ctx, query)
}

func (s securedReadSession) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	resources := resourcesForQuery(query)
	if err := s.guard.authorizeRequest(ctx, Request{Operation: Query, Resources: resources, Query: query}); err != nil {
		return nil, err
	}
	return s.session.ExecuteQueryToRecordsetReader(ctx, query, options...)
}

type securedWriteSession struct {
	session dal.WriteSession
	guard   guard
}

func (s securedWriteSession) Set(ctx context.Context, record record.Record) error {
	if err := s.guard.authorize(ctx, Set, RecordResourceForKey(record.Key())); err != nil {
		return err
	}
	return s.session.Set(ctx, record)
}

func (s securedWriteSession) SetMulti(ctx context.Context, records []record.Record) error {
	if err := s.guard.authorize(ctx, Set, resourcesForRecords(records)...); err != nil {
		return err
	}
	return s.session.SetMulti(ctx, records)
}

func (s securedWriteSession) Insert(ctx context.Context, record record.Record, options ...dal.InsertOption) error {
	if err := s.guard.authorize(ctx, Insert, RecordResourceForKey(record.Key())); err != nil {
		return err
	}
	return s.session.Insert(ctx, record, options...)
}

func (s securedWriteSession) InsertMulti(ctx context.Context, records []record.Record, options ...dal.InsertOption) error {
	if err := s.guard.authorize(ctx, Insert, resourcesForRecords(records)...); err != nil {
		return err
	}
	return s.session.InsertMulti(ctx, records, options...)
}

func (s securedWriteSession) Update(ctx context.Context, key *record.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	if err := s.guard.authorize(ctx, Update, RecordResourceForKey(key)); err != nil {
		return err
	}
	return s.session.Update(ctx, key, updates, preconditions...)
}

func (s securedWriteSession) UpdateRecord(ctx context.Context, record record.Record, updates []update.Update, preconditions ...dal.Precondition) error {
	if err := s.guard.authorize(ctx, Update, RecordResourceForKey(record.Key())); err != nil {
		return err
	}
	return s.session.UpdateRecord(ctx, record, updates, preconditions...)
}

func (s securedWriteSession) UpdateMulti(ctx context.Context, keys []*record.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	if err := s.guard.authorize(ctx, Update, resourcesForKeys(keys)...); err != nil {
		return err
	}
	return s.session.UpdateMulti(ctx, keys, updates, preconditions...)
}

func (s securedWriteSession) Delete(ctx context.Context, key *record.Key) error {
	if err := s.guard.authorize(ctx, Delete, RecordResourceForKey(key)); err != nil {
		return err
	}
	return s.session.Delete(ctx, key)
}

func (s securedWriteSession) DeleteMulti(ctx context.Context, keys []*record.Key) error {
	if err := s.guard.authorize(ctx, Delete, resourcesForKeys(keys)...); err != nil {
		return err
	}
	return s.session.DeleteMulti(ctx, keys)
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
		return CollectionResourceFor(source.Parent(), source.Name())
	case *dal.CollectionRef:
		return CollectionResourceFor(source.Parent(), source.Name())
	case dal.CollectionGroupRef:
		return CollectionGroup(source.Name())
	case *dal.CollectionGroupRef:
		return CollectionGroup(source.Name())
	default:
		return OpaqueQuery(fmt.Sprint(source))
	}
}
