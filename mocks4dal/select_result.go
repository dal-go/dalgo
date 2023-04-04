package mocks4dal

import (
	"encoding/json"
	"github.com/dal-go/dalgo/dal"
)

// SelectResult is a helper class that can be used in test definitions (TT)
type SelectResult struct {
	Reader func(into func() interface{}) dal.Reader
	Err    error
}

// NewSelectResult creates new SelectResult
func NewSelectResult(getReader func(into func() interface{}) dal.Reader, err error) SelectResult {
	if getReader == nil && err == nil {
		panic("getReader == nil && Err == nil")
	}
	return SelectResult{Reader: func(into func() interface{}) dal.Reader {
		if getReader == nil {
			return nil
		}
		return getReader(into)
	}, Err: err}
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

// NewSingleRecordReader creates a reader that returns a single record
func NewSingleRecordReader(key *dal.Key, data string, into func() interface{}) *singleRecordReader {
	return &singleRecordReader{key: key, data: data, into: into}
}
