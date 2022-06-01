package mock_dal

import (
	"context"
	"encoding/json"
	"github.com/strongo/dalgo/dal"
)

type SelectResult struct {
	reader func(into func() interface{}) dal.Reader
	err    error
}

func NewSelectResult(getReader func(into func() interface{}) dal.Reader, err error) SelectResult {
	if getReader == nil && err == nil {
		panic("getReader == nil && err == nil")
	}
	return SelectResult{reader: func(into func() interface{}) dal.Reader {
		if getReader == nil {
			return nil
		}
		return getReader(into)
	}, err: err}
}

type singleRecordReader struct {
	key  *dal.Key
	data string
	into func() interface{}
	i    int
}

func (s *singleRecordReader) Next() (dal.Record, error) {
	if s.i > 0 {
		return nil, dal.ErrNoMoreRecords
	}
	s.i++
	if s.data == "" {
		panic("singleRecordReader.data is empty")
	}
	data := s.into()
	err := json.Unmarshal([]byte(s.data), data)
	if err != nil {
		return nil, err
	}
	return dal.NewRecordWithoutKey(data), err
}

var _ dal.Reader = (*singleRecordReader)(nil)

func NewSingleRecordReader(key *dal.Key, data string, into func() interface{}) *singleRecordReader {
	return &singleRecordReader{key: key, data: data, into: into}
}

type dbMock struct {
	readonlySession
}

var _ dal.Database = (*dbMock)(nil)
var _ dal.ReadSession = (*dbMock)(nil)

func (db dbMock) RunReadonlyTransaction(ctx context.Context, f dal.ROTxWorker, options ...dal.TransactionOption) error {
	//TODO implement me
	panic("implement me")
}

func (db dbMock) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, options ...dal.TransactionOption) error {
	txCtx := context.WithValue(ctx, "dalgo_tx", true)
	return f(txCtx, readwriteTransaction{})
}

func NewDbMock() dbMock {
	return dbMock{readonlySession{onSelectFrom: make(map[string]SelectResult)}}
}
