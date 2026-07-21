package ddl

import (
	"context"
	"errors"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

// minStubAdapter implements dal.Adapter with a controllable name.
type minStubAdapter struct{ name string }

func (s minStubAdapter) Name() string    { return s.name }
func (s minStubAdapter) Version() string { return "" }

// minStubDB is a minimal dal.DB stub used by tests in this package.
// All methods except Adapter() and ID() return errors or panic —
// tests that need more functionality wrap or embed this. Embeds
// dal.NoConcurrency to satisfy the ConcurrencyAware composition.
type minStubDB struct {
	dal.NoConcurrency
	adapter dal.Adapter
}

func newMinStubDB(name string) *minStubDB {
	return &minStubDB{adapter: minStubAdapter{name: name}}
}

// newMinStubDBNilAdapter creates a stub whose Adapter() returns nil.
func newMinStubDBNilAdapter() *minStubDB {
	return &minStubDB{adapter: nil}
}

func (s *minStubDB) ID() string           { return "stub" }
func (s *minStubDB) Adapter() dal.Adapter { return s.adapter }
func (s *minStubDB) Schema() dal.Schema   { return nil }
func (s *minStubDB) RunReadonlyTransaction(_ context.Context, _ dal.ROTxWorker, _ ...dal.TransactionOption) error {
	return errors.New("not used in tests")
}
func (s *minStubDB) RunReadwriteTransaction(_ context.Context, _ dal.RWTxWorker, _ ...dal.TransactionOption) error {
	return errors.New("not used in tests")
}
func (s *minStubDB) Get(_ context.Context, _ record.Record) error { return errors.New("not used") }
func (s *minStubDB) Exists(_ context.Context, _ *record.Key) (bool, error) {
	return false, errors.New("not used")
}
func (s *minStubDB) GetMulti(_ context.Context, _ []record.Record) error {
	return errors.New("not used")
}
func (s *minStubDB) ExecuteQueryToRecordsReader(_ context.Context, _ dal.Query) (dal.RecordsReader, error) {
	return nil, errors.New("not used")
}
func (s *minStubDB) ExecuteQueryToRecordsetReader(_ context.Context, _ dal.Query, _ ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, errors.New("not used")
}
