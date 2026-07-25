package dal

import (
	"context"
	"fmt"

	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

// writePipeline is the framework-owned write path. Every write a caller can
// reach through a DB built by NewDB goes through it, so the invariants DALgo
// declares are enforced before a storage adapter is entered rather than by each
// adapter in turn.
//
// Record-bearing writes (Insert, Set, UpdateRecord and their Multi variants)
// run BeforeSave: validation plus the registered before-save hooks. Deletes run
// BeforeDelete: hooks only, since a delete carries no data to validate.
// Key-only updates carry no data either and have no hook registry of their own,
// so they pass straight through — the pipeline is where that would change.
type writePipeline struct {
	ws       WriteSession
	db       DB
	validate bool
}

var _ WriteSession = writePipeline{}

func (p writePipeline) dalgoWithoutValidation() WriteSession {
	p.validate = false
	return p
}

// beforeSave runs the pipeline for one record-bearing write. With validation
// disabled (see WithoutValidation) the hooks still run — skipping validation is
// not the same as leaving the pipeline.
func (p writePipeline) beforeSave(ctx context.Context, r record.Record) error {
	if p.validate {
		return BeforeSave(ctx, p.db, r)
	}
	return callRecordHooks(ctx, r, registeredHooks(&beforeSaveHooks))
}

// beforeSaveAll runs the pipeline for every record before any of them is
// written, so a batch containing one invalid record is rejected whole rather
// than half-applied.
func (p writePipeline) beforeSaveAll(ctx context.Context, records []record.Record) error {
	for i, r := range records {
		if err := p.beforeSave(ctx, r); err != nil {
			return fmt.Errorf("record at index %d: %w", i, err)
		}
	}
	return nil
}

func (p writePipeline) beforeDeleteAll(ctx context.Context, keys []*record.Key) error {
	for i, key := range keys {
		if err := BeforeDelete(ctx, p.db, key); err != nil {
			return fmt.Errorf("key at index %d: %w", i, err)
		}
	}
	return nil
}

func (p writePipeline) Insert(ctx context.Context, r record.Record, opts ...InsertOption) error {
	if err := p.beforeSave(ctx, r); err != nil {
		return err
	}
	return p.ws.Insert(ctx, r, opts...)
}

func (p writePipeline) InsertMulti(ctx context.Context, records []record.Record, opts ...InsertOption) error {
	if err := p.beforeSaveAll(ctx, records); err != nil {
		return err
	}
	return p.ws.InsertMulti(ctx, records, opts...)
}

func (p writePipeline) Set(ctx context.Context, r record.Record) error {
	if err := p.beforeSave(ctx, r); err != nil {
		return err
	}
	return p.ws.Set(ctx, r)
}

func (p writePipeline) SetMulti(ctx context.Context, records []record.Record) error {
	if err := p.beforeSaveAll(ctx, records); err != nil {
		return err
	}
	return p.ws.SetMulti(ctx, records)
}

// UpdateRecord carries record data, so it is validated like an insert or a set.
func (p writePipeline) UpdateRecord(ctx context.Context, r record.Record, updates []update.Update, preconditions ...Precondition) error {
	if err := p.beforeSave(ctx, r); err != nil {
		return err
	}
	return p.ws.UpdateRecord(ctx, r, updates, preconditions...)
}

// Update addresses a record by key and carries no record data, so there is
// nothing to validate. It stays on the pipeline so the write path is uniform.
func (p writePipeline) Update(ctx context.Context, key *record.Key, updates []update.Update, preconditions ...Precondition) error {
	return p.ws.Update(ctx, key, updates, preconditions...)
}

// UpdateMulti addresses records by key and carries no record data — see Update.
func (p writePipeline) UpdateMulti(ctx context.Context, keys []*record.Key, updates []update.Update, preconditions ...Precondition) error {
	return p.ws.UpdateMulti(ctx, keys, updates, preconditions...)
}

func (p writePipeline) Delete(ctx context.Context, key *record.Key) error {
	if err := BeforeDelete(ctx, p.db, key); err != nil {
		return err
	}
	return p.ws.Delete(ctx, key)
}

func (p writePipeline) DeleteMulti(ctx context.Context, keys []*record.Key) error {
	if err := p.beforeDeleteAll(ctx, keys); err != nil {
		return err
	}
	return p.ws.DeleteMulti(ctx, keys)
}

// WithoutValidation returns a write session that performs writes without
// validating record data. Registered hooks still run: this opts out of
// validation, not out of the pipeline.
//
// It exists because some writes legitimately must skip validation — repair
// migrations, bulk import, writing a record that is known invalid in order to
// fix it later. The point is not to permit skipping but to make it visible and
// greppable at the call site instead of invisible inside an adapter:
//
//	if err := dal.WithoutValidation(tx).Set(ctx, rec); err != nil {
//
// A session that is not framework-managed has no validation to skip and is
// returned unchanged.
func WithoutValidation(s WriteSession) WriteSession {
	if u, ok := s.(interface{ dalgoWithoutValidation() WriteSession }); ok {
		return u.dalgoWithoutValidation()
	}
	return s
}
