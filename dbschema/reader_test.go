package dbschema

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/stretchr/testify/assert"
)

// stubAdapter implements dal.Adapter with a controllable name.
type stubAdapter struct{ name string }

func (s stubAdapter) Name() string    { return s.name }
func (s stubAdapter) Version() string { return "" }

// stubDB is a minimal dal.DB stub. Most methods return errors;
// tests use only the ones they need. Embeds dal.NoConcurrency to
// satisfy that embedded interface on dal.DB.
type stubDB struct {
	dal.NoConcurrency
	adapter dal.Adapter
}

func (s *stubDB) ID() string           { return "stub" }
func (s *stubDB) Adapter() dal.Adapter { return s.adapter }
func (s *stubDB) Schema() dal.Schema   { return nil }
func (s *stubDB) RunReadonlyTransaction(_ context.Context, _ dal.ROTxWorker, _ ...dal.TransactionOption) error {
	return errors.New("not used in tests")
}
func (s *stubDB) RunReadwriteTransaction(_ context.Context, _ dal.RWTxWorker, _ ...dal.TransactionOption) error {
	return errors.New("not used in tests")
}
func (s *stubDB) Get(_ context.Context, _ dal.Record) error { return errors.New("not used") }
func (s *stubDB) Exists(_ context.Context, _ *dal.Key) (bool, error) {
	return false, errors.New("not used")
}
func (s *stubDB) GetMulti(_ context.Context, _ []dal.Record) error { return errors.New("not used") }
func (s *stubDB) ExecuteQueryToRecordsReader(_ context.Context, _ dal.Query) (dal.RecordsReader, error) {
	return nil, errors.New("not used")
}
func (s *stubDB) ExecuteQueryToRecordsetReader(_ context.Context, _ dal.Query, _ ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, errors.New("not used")
}

// recordingReader implements SchemaReader and captures the last call.
type recordingReader struct {
	lastOp  string
	lastArg any
}

func (r *recordingReader) ListCollections(_ context.Context, parent *dal.Key) ([]dal.CollectionRef, error) {
	r.lastOp = "ListCollections"
	r.lastArg = parent
	return nil, nil
}
func (r *recordingReader) DescribeCollection(_ context.Context, ref *dal.CollectionRef) (*CollectionDef, error) {
	r.lastOp = "DescribeCollection"
	r.lastArg = ref
	return &CollectionDef{Name: "users"}, nil
}
func (r *recordingReader) ListIndexes(_ context.Context, ref *dal.CollectionRef) ([]IndexDef, error) {
	r.lastOp = "ListIndexes"
	r.lastArg = ref
	return nil, nil
}
func (r *recordingReader) ListConstraints(_ context.Context, ref *dal.CollectionRef) ([]ConstraintDef, error) {
	r.lastOp = "ListConstraints"
	r.lastArg = ref
	return nil, nil
}
func (r *recordingReader) ListReferrers(_ context.Context, ref *dal.CollectionRef) ([]Referrer, error) {
	r.lastOp = "ListReferrers"
	r.lastArg = ref
	return nil, nil
}

// readerStubDB embeds stubDB AND a recordingReader so it satisfies
// both dal.DB and dbschema.SchemaReader.
type readerStubDB struct {
	*stubDB
	*recordingReader
}

func newReaderStubDB(name string) *readerStubDB {
	return &readerStubDB{
		stubDB:          &stubDB{adapter: stubAdapter{name: name}},
		recordingReader: &recordingReader{},
	}
}

func TestSchemaReader_InterfaceExists(t *testing.T) {
	// Per REQ:schema-reader-interface AC-1 + AC-2.
	var _ SchemaReader = (*recordingReader)(nil)
}

func TestConstraintDef_Compiles(t *testing.T) {
	// Per REQ:supporting-types AC-1.
	c := ConstraintDef{Name: "uq_email", Type: "unique"}
	_ = c
}

func TestReferrer_Compiles(t *testing.T) {
	// Per REQ:supporting-types AC-1.
	r := Referrer{
		Collection: dal.CollectionRef{},
		Fields:     []dal.FieldName{"user_id"},
	}
	_ = r
}

func TestHelpers_Compile(t *testing.T) {
	// Per REQ:helper-functions AC-1.
	_ = ListCollections
	_ = DescribeCollection
	_ = ListIndexes
	_ = ListConstraints
	_ = ListReferrers
}

func TestDescribeCollection_Dispatches(t *testing.T) {
	// Per REQ:helper-functions AC-2.
	db := newReaderStubDB("stub-driver")
	ref := &dal.CollectionRef{}
	result, err := DescribeCollection(context.Background(), db, ref)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "DescribeCollection", db.lastOp)
	assert.Equal(t, ref, db.lastArg)
}

func TestDescribeCollection_NotImplementer(t *testing.T) {
	// Per REQ:helper-functions AC-3.
	db := &stubDB{adapter: stubAdapter{name: "no-reader"}}
	_, err := DescribeCollection(context.Background(), db, &dal.CollectionRef{})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, dal.ErrNotSupported))
	var ue *NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "DescribeCollection", ue.Op)
	assert.Equal(t, "no-reader", ue.Backend)
}
