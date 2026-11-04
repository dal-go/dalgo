package dbschema

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
)

type nativeAccessStubDB struct {
	*stubDB
}

func (s *nativeAccessStubDB) ReadNativeAccess(_ context.Context, ref *dal.CollectionRef) (*NativeAccess, error) {
	return &NativeAccess{Collection: *ref, Coverage: NativeAccessCoverage{Level: NativeCoveragePartial}}, nil
}

func TestNativeAccessReader_IsOptional(t *testing.T) {
	var _ NativeAccessReader = (*nativeAccessStubDB)(nil)
	_, ok := dal.As[NativeAccessReader](dal.NewDB(&stubDB{adapter: stubAdapter{name: "no-native"}}))
	assert.False(t, ok, "a driver without the capability does not satisfy it")
	db := dal.NewDB(&nativeAccessStubDB{stubDB: &stubDB{adapter: stubAdapter{name: "native"}}})
	r, ok := dal.As[NativeAccessReader](db)
	assert.True(t, ok)
	ref := dal.NewRootCollectionRef("invoices", "")
	got, err := r.ReadNativeAccess(context.Background(), &ref)
	assert.NoError(t, err)
	assert.Equal(t, NativeCoveragePartial, got.Coverage.Level)
}
