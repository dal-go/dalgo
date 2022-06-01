package mock_dal

import (
	"context"
	"github.com/strongo/dalgo/dal"
)

type readwriteTransaction struct {
	readonlySession
}

func (d readwriteTransaction) Options() dal.TransactionOptions {
	//TODO implement me
	panic("implement me")
}

func (d readwriteTransaction) Insert(c context.Context, record dal.Record, opts ...dal.InsertOption) error {
	//TODO implement me
	panic("implement me")
}

func (d readwriteTransaction) Set(ctx context.Context, record dal.Record) error {
	//TODO implement me
	panic("implement me")
}

func (d readwriteTransaction) SetMulti(ctx context.Context, records []dal.Record) error {
	//TODO implement me
	panic("implement me")
}

func (d readwriteTransaction) Update(ctx context.Context, key *dal.Key, updates []dal.Update, preconditions ...dal.Precondition) error {
	//TODO implement me
	panic("implement me")
}

func (d readwriteTransaction) UpdateMulti(c context.Context, keys []*dal.Key, updates []dal.Update, preconditions ...dal.Precondition) error {
	//TODO implement me
	panic("implement me")
}

func (d readwriteTransaction) Delete(ctx context.Context, key *dal.Key) error {
	//TODO implement me
	panic("implement me")
}

func (d readwriteTransaction) DeleteMulti(ctx context.Context, keys []*dal.Key) error {
	//TODO implement me
	panic("implement me")
}

var _ dal.ReadwriteTransaction = (*readwriteTransaction)(nil)

func (d readwriteTransaction) Get(ctx context.Context, record dal.Record) error {
	//TODO implement me
	panic("implement me")
}

func (d readwriteTransaction) GetMulti(ctx context.Context, records []dal.Record) error {
	//TODO implement me
	panic("implement me")
}

func (d readwriteTransaction) Select(ctx context.Context, query dal.Select) (dal.Reader, error) {
	collectionPath := query.From.Path()
	result := d.onSelectFrom[collectionPath]
	return result.reader(query.Into), result.err
}
