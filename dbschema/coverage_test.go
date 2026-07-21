package dbschema

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// TestType_String_Unknown exercises the default arm of Type.String for
// unrecognized values to hit 100% coverage on the switch.
func TestType_String_Unknown(t *testing.T) {
	got := Type(99).String()
	assert.Equal(t, "unknown", got)
}

// TestBackendName_NilDB covers the early-return branch when db is nil.
func TestBackendName_NilDB(t *testing.T) {
	got := backendName(nil)
	assert.Equal(t, "", got)
}

// TestBackendName_NilAdapter covers the branch where db.Adapter() is nil.
func TestBackendName_NilAdapter(t *testing.T) {
	got := backendName(&stubDB{adapter: nil})
	assert.Equal(t, "", got)
}

// TestDefaultLiteral_DefaultExpr covers the sealed marker method.
func TestDefaultLiteral_DefaultExpr(t *testing.T) {
	var e DefaultExpr = DefaultLiteral{Value: 42}
	assert.NotNil(t, e)
	DefaultLiteral{}.defaultExpr()
}

// TestDefaultCurrentTimestamp_DefaultExpr covers the sealed marker method.
func TestDefaultCurrentTimestamp_DefaultExpr(t *testing.T) {
	var e DefaultExpr = DefaultCurrentTimestamp{}
	assert.NotNil(t, e)
	DefaultCurrentTimestamp{}.defaultExpr()
}

// ----- Helper-function dispatch + not-implementer tests for the four
// remaining SchemaReader helpers (DescribeCollection is already covered).

func TestListCollections_Dispatches(t *testing.T) {
	db := newReaderStubDB("stub-driver")
	parent := &record.Key{}
	_, err := ListCollections(context.Background(), db, parent)
	assert.NoError(t, err)
	assert.Equal(t, "ListCollections", db.lastOp)
	assert.Equal(t, parent, db.lastArg)
}

func TestListCollections_NotImplementer(t *testing.T) {
	db := &stubDB{adapter: stubAdapter{name: "no-reader"}}
	_, err := ListCollections(context.Background(), db, nil)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, dal.ErrNotSupported))
	var ue *NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "ListCollections", ue.Op)
	assert.Equal(t, "no-reader", ue.Backend)
}

func TestListIndexes_Dispatches(t *testing.T) {
	db := newReaderStubDB("stub-driver")
	ref := &dal.CollectionRef{}
	_, err := ListIndexes(context.Background(), db, ref)
	assert.NoError(t, err)
	assert.Equal(t, "ListIndexes", db.lastOp)
	assert.Equal(t, ref, db.lastArg)
}

func TestListIndexes_NotImplementer(t *testing.T) {
	db := &stubDB{adapter: stubAdapter{name: "no-reader"}}
	_, err := ListIndexes(context.Background(), db, &dal.CollectionRef{})
	assert.Error(t, err)
	var ue *NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "ListIndexes", ue.Op)
}

func TestListConstraints_Dispatches(t *testing.T) {
	db := newReaderStubDB("stub-driver")
	ref := &dal.CollectionRef{}
	_, err := ListConstraints(context.Background(), db, ref)
	assert.NoError(t, err)
	assert.Equal(t, "ListConstraints", db.lastOp)
	assert.Equal(t, ref, db.lastArg)
}

func TestListConstraints_NotImplementer(t *testing.T) {
	db := &stubDB{adapter: stubAdapter{name: "no-reader"}}
	_, err := ListConstraints(context.Background(), db, &dal.CollectionRef{})
	assert.Error(t, err)
	var ue *NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "ListConstraints", ue.Op)
}

func TestListReferrers_Dispatches(t *testing.T) {
	db := newReaderStubDB("stub-driver")
	ref := &dal.CollectionRef{}
	_, err := ListReferrers(context.Background(), db, ref)
	assert.NoError(t, err)
	assert.Equal(t, "ListReferrers", db.lastOp)
	assert.Equal(t, ref, db.lastArg)
}

func TestListReferrers_NotImplementer(t *testing.T) {
	db := &stubDB{adapter: stubAdapter{name: "no-reader"}}
	_, err := ListReferrers(context.Background(), db, &dal.CollectionRef{})
	assert.Error(t, err)
	var ue *NotSupportedError
	assert.True(t, errors.As(err, &ue))
	assert.Equal(t, "ListReferrers", ue.Op)
}
