package dal_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

// errInvalidTestRecord is what the fixture's Validate returns. Tests assert on
// it with errors.Is so that "the error surfaced is the validation error, not a
// storage error" is checked by identity rather than by message.
var errInvalidTestRecord = errors.New("test: record is invalid")

// errStorageFailed is what the spy backend returns when a test asks it to fail.
// It must never be what a caller sees for an invalid record.
var errStorageFailed = errors.New("test: storage failed")

// validatableData implements dal.ValidatableRecord. It is invalid when Name is
// empty, so a fixture's validity is visible in its literal.
type validatableData struct {
	Name string
}

func (d validatableData) Validate() error {
	if d.Name == "" {
		return errInvalidTestRecord
	}
	return nil
}

// plainData deliberately does NOT implement dal.ValidatableRecord.
type plainData struct {
	Name string
}

func validRecord(id string) record.Record {
	return record.NewRecordWithData(record.NewKeyWithID("things", id), &validatableData{Name: id})
}

func invalidRecord(id string) record.Record {
	return record.NewRecordWithData(record.NewKeyWithID("things", id), &validatableData{})
}

func plainRecord(id string) record.Record {
	return record.NewRecordWithData(record.NewKeyWithID("things", id), &plainData{Name: id})
}

// spyWriteSession records every write that reaches it. "Reached the backend" is
// the only honest way to assert that validation ran BEFORE the adapter, so the
// pipeline tests assert on this rather than on the returned error alone.
type spyWriteSession struct {
	writes []string
	fail   error
}

func (s *spyWriteSession) record(op string, n int) error {
	s.writes = append(s.writes, fmt.Sprintf("%s(%d)", op, n))
	return s.fail
}

func (s *spyWriteSession) Insert(_ context.Context, _ record.Record, _ ...dal.InsertOption) error {
	return s.record("Insert", 1)
}

func (s *spyWriteSession) InsertMulti(_ context.Context, records []record.Record, _ ...dal.InsertOption) error {
	return s.record("InsertMulti", len(records))
}

func (s *spyWriteSession) Set(_ context.Context, _ record.Record) error {
	return s.record("Set", 1)
}

func (s *spyWriteSession) SetMulti(_ context.Context, records []record.Record) error {
	return s.record("SetMulti", len(records))
}

func (s *spyWriteSession) Update(_ context.Context, _ *record.Key, _ []update.Update, _ ...dal.Precondition) error {
	return s.record("Update", 1)
}

func (s *spyWriteSession) UpdateRecord(_ context.Context, _ record.Record, _ []update.Update, _ ...dal.Precondition) error {
	return s.record("UpdateRecord", 1)
}

func (s *spyWriteSession) UpdateMulti(_ context.Context, keys []*record.Key, _ []update.Update, _ ...dal.Precondition) error {
	return s.record("UpdateMulti", len(keys))
}

func (s *spyWriteSession) Delete(_ context.Context, _ *record.Key) error {
	return s.record("Delete", 1)
}

func (s *spyWriteSession) DeleteMulti(_ context.Context, keys []*record.Key) error {
	return s.record("DeleteMulti", len(keys))
}

var _ dal.WriteSession = (*spyWriteSession)(nil)

// spyTx is a read-write transaction whose writes are recorded by the shared spy.
type spyTx struct {
	*spyWriteSession
}

func (spyTx) ID() string                      { return "spy-tx" }
func (spyTx) Options() dal.TransactionOptions { return dal.NewTransactionOptions() }
func (spyTx) Get(context.Context, record.Record) error {
	return dal.ErrNotSupported
}
func (spyTx) Exists(context.Context, *record.Key) (bool, error) {
	return false, dal.ErrNotSupported
}
func (spyTx) GetMulti(context.Context, []record.Record) error { return dal.ErrNotSupported }
func (spyTx) ExecuteQueryToRecordsReader(context.Context, dal.Query) (dal.RecordsReader, error) {
	return nil, dal.ErrNotSupported
}
func (spyTx) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, dal.ErrNotSupported
}

var _ dal.ReadwriteTransaction = spyTx{}

// spyBackend is a dal.Backend whose transactions and database-level writes both
// land in the same spy, so a test can assert "the backend was never touched"
// whichever write path it exercised.
type spyBackend struct {
	dal.NoConcurrency
	*spyWriteSession
}

func newSpyBackend() *spyBackend {
	return &spyBackend{spyWriteSession: &spyWriteSession{}}
}

func (spyBackend) ID() string           { return "spy" }
func (spyBackend) Adapter() dal.Adapter { return dal.NewAdapter("spy", "0") }
func (spyBackend) Schema() dal.Schema   { return nil }

func (b *spyBackend) RunReadonlyTransaction(ctx context.Context, f dal.ROTxWorker, _ ...dal.TransactionOption) error {
	return f(ctx, spyTx{spyWriteSession: b.spyWriteSession})
}

func (b *spyBackend) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, _ ...dal.TransactionOption) error {
	return f(ctx, spyTx{spyWriteSession: b.spyWriteSession})
}

func (b *spyBackend) Get(context.Context, record.Record) error { return dal.ErrNotSupported }
func (b *spyBackend) Exists(context.Context, *record.Key) (bool, error) {
	return false, dal.ErrNotSupported
}
func (b *spyBackend) GetMulti(context.Context, []record.Record) error { return dal.ErrNotSupported }
func (b *spyBackend) ExecuteQueryToRecordsReader(context.Context, dal.Query) (dal.RecordsReader, error) {
	return nil, dal.ErrNotSupported
}
func (b *spyBackend) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, dal.ErrNotSupported
}

var (
	_ dal.Backend      = (*spyBackend)(nil)
	_ dal.WriteSession = (*spyBackend)(nil)
)

// readOnlyBackend does NOT implement dal.WriteSession. It exists to prove NewDB
// does not invent a database-level write surface an adapter never had.
type readOnlyBackend struct {
	dal.NoConcurrency
}

func (readOnlyBackend) ID() string           { return "read-only" }
func (readOnlyBackend) Adapter() dal.Adapter { return dal.NewAdapter("read-only", "0") }
func (readOnlyBackend) Schema() dal.Schema   { return nil }
func (readOnlyBackend) RunReadonlyTransaction(context.Context, dal.ROTxWorker, ...dal.TransactionOption) error {
	return dal.ErrNotSupported
}
func (readOnlyBackend) RunReadwriteTransaction(context.Context, dal.RWTxWorker, ...dal.TransactionOption) error {
	return dal.ErrNotSupported
}
func (readOnlyBackend) Get(context.Context, record.Record) error { return dal.ErrNotSupported }
func (readOnlyBackend) Exists(context.Context, *record.Key) (bool, error) {
	return false, dal.ErrNotSupported
}
func (readOnlyBackend) GetMulti(context.Context, []record.Record) error { return dal.ErrNotSupported }
func (readOnlyBackend) ExecuteQueryToRecordsReader(context.Context, dal.Query) (dal.RecordsReader, error) {
	return nil, dal.ErrNotSupported
}
func (readOnlyBackend) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, dal.ErrNotSupported
}

var _ dal.Backend = readOnlyBackend{}
