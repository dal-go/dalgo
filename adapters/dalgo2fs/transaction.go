package dalgo2fs

import (
	"context"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

var _ dal.ReadwriteTransaction = (*transaction)(nil)

type transaction struct {
}

// ID returns an empty transaction id.
func (t transaction) ID() string {
	return ""
}

// Options returns nil.
func (t transaction) Options() dal.TransactionOptions {
	return nil
}

// Get returns dal.ErrNotImplementedYet.
func (t transaction) Get(_ context.Context, _ record.Record) error {
	return dal.ErrNotImplementedYet
}

// Exists returns dal.ErrNotImplementedYet.
func (t transaction) Exists(_ context.Context, _ *record.Key) (bool, error) {
	return false, dal.ErrNotImplementedYet
}

// GetMulti returns dal.ErrNotImplementedYet.
func (t transaction) GetMulti(_ context.Context, _ []record.Record) error {
	return dal.ErrNotImplementedYet
}

// ExecuteQueryToRecordsReader returns dal.ErrNotImplementedYet.
func (t transaction) ExecuteQueryToRecordsReader(_ context.Context, _ dal.Query) (dal.RecordsReader, error) {
	return nil, dal.ErrNotImplementedYet
}

// ExecuteQueryToRecordsetReader returns dal.ErrNotImplementedYet.
func (t transaction) ExecuteQueryToRecordsetReader(_ context.Context, _ dal.Query, _ ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, dal.ErrNotImplementedYet
}

// Set returns dal.ErrNotSupported.
func (t transaction) Set(_ context.Context, _ record.Record) error {
	return dal.ErrNotSupported
}

// SetMulti returns dal.ErrNotSupported.
func (t transaction) SetMulti(_ context.Context, _ []record.Record) error {
	return dal.ErrNotSupported
}

// Delete returns dal.ErrNotImplementedYet.
func (t transaction) Delete(_ context.Context, _ *record.Key) error {
	return dal.ErrNotImplementedYet
}

// DeleteMulti returns dal.ErrNotImplementedYet.
func (t transaction) DeleteMulti(_ context.Context, _ []*record.Key) error {
	return dal.ErrNotImplementedYet
}

// Update returns dal.ErrNotSupported.
func (t transaction) Update(_ context.Context, _ *record.Key, _ []update.Update, _ ...dal.Precondition) error {
	return dal.ErrNotSupported
}

// UpdateRecord returns dal.ErrNotSupported.
func (t transaction) UpdateRecord(_ context.Context, _ record.Record, _ []update.Update, _ ...dal.Precondition) error {
	return dal.ErrNotSupported
}

// UpdateMulti returns dal.ErrNotSupported.
func (t transaction) UpdateMulti(_ context.Context, _ []*record.Key, _ []update.Update, _ ...dal.Precondition) error {
	return dal.ErrNotSupported
}

// Insert returns dal.ErrNotImplementedYet.
func (t transaction) Insert(_ context.Context, _ record.Record, _ ...dal.InsertOption) error {
	return dal.ErrNotImplementedYet
}

// InsertMulti returns dal.ErrNotImplementedYet.
func (t transaction) InsertMulti(_ context.Context, _ []record.Record, _ ...dal.InsertOption) error {
	return dal.ErrNotImplementedYet
}
