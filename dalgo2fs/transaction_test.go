package dalgo2fs

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/update"
	"github.com/stretchr/testify/assert"
)

func TestTransaction(t *testing.T) {
	tx := transaction{}
	ctx := context.Background()

	assert.Equal(t, "", tx.ID())
	assert.Nil(t, tx.Options())

	assert.Equal(t, dal.ErrNotSupported, tx.Get(ctx, nil))
	exists, err := tx.Exists(ctx, nil)
	assert.False(t, exists)
	assert.Equal(t, dal.ErrNotSupported, err)
	assert.Equal(t, dal.ErrNotSupported, tx.GetMulti(ctx, nil))
	rr, err := tx.GetRecordsReader(ctx, nil)
	assert.Nil(t, rr)
	assert.Equal(t, dal.ErrNotSupported, err)
	rsr, err := tx.GetRecordsetReader(ctx, nil, nil)
	assert.Nil(t, rsr)
	assert.Equal(t, dal.ErrNotSupported, err)
	assert.Equal(t, dal.ErrNotSupported, tx.Set(ctx, nil))
	assert.Equal(t, dal.ErrNotSupported, tx.SetMulti(ctx, nil))
	assert.Equal(t, dal.ErrNotSupported, tx.Delete(ctx, nil))
	assert.Equal(t, dal.ErrNotSupported, tx.DeleteMulti(ctx, nil))
	assert.Equal(t, dal.ErrNotSupported, tx.Update(ctx, nil, []update.Update{}))
	assert.Equal(t, dal.ErrNotSupported, tx.UpdateRecord(ctx, nil, []update.Update{}))
	assert.Equal(t, dal.ErrNotSupported, tx.UpdateMulti(ctx, nil, []update.Update{}))
	assert.Equal(t, dal.ErrNotSupported, tx.Insert(ctx, nil))
	assert.Equal(t, dal.ErrNotSupported, tx.InsertMulti(ctx, nil))
}
