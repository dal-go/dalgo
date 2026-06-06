package dalgo2fs

import (
	"context"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/dalgo/update"
)

var _ dal.ReadwriteTransaction = (*transaction)(nil)

type transaction struct {
}

func (t transaction) ID() string {
	return ""
}

func (t transaction) Options() dal.TransactionOptions {
	return nil
}

func (t transaction) Get(_ context.Context, _ dal.Record) error {
	return dal.ErrNotImplementedYet
}

func (t transaction) Exists(_ context.Context, _ *dal.Key) (bool, error) {
	return false, dal.ErrNotImplementedYet
}

func (t transaction) GetMulti(_ context.Context, _ []dal.Record) error {
	return dal.ErrNotImplementedYet
}

func (t transaction) ExecuteQueryToRecordsReader(_ context.Context, _ dal.Query) (dal.RecordsReader, error) {
	return nil, dal.ErrNotImplementedYet
}

func (t transaction) ExecuteQueryToRecordsetReader(_ context.Context, _ dal.Query, _ ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, dal.ErrNotImplementedYet
}

func (t transaction) Set(_ context.Context, _ dal.Record) error {
	return dal.ErrNotSupported
}

func (t transaction) SetMulti(_ context.Context, _ []dal.Record) error {
	return dal.ErrNotSupported
}

func (t transaction) Delete(_ context.Context, _ *dal.Key) error {
	return dal.ErrNotImplementedYet
}

func (t transaction) DeleteMulti(_ context.Context, _ []*dal.Key) error {
	return dal.ErrNotImplementedYet
}

func (t transaction) Update(_ context.Context, _ *dal.Key, _ []update.Update, _ ...dal.Precondition) error {
	return dal.ErrNotSupported
}

func (t transaction) UpdateRecord(_ context.Context, _ dal.Record, _ []update.Update, _ ...dal.Precondition) error {
	return dal.ErrNotSupported
}

func (t transaction) UpdateMulti(_ context.Context, _ []*dal.Key, _ []update.Update, _ ...dal.Precondition) error {
	return dal.ErrNotSupported
}

func (t transaction) Insert(_ context.Context, _ dal.Record, _ ...dal.InsertOption) error {
	return dal.ErrNotImplementedYet
}

func (t transaction) InsertMulti(_ context.Context, _ []dal.Record, _ ...dal.InsertOption) error {
	return dal.ErrNotImplementedYet
}
