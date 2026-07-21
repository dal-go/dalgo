package dal_test

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
	"github.com/stretchr/testify/assert"
)

func TestApplyChangesExecutesAndResetsChangeSet(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	key := record.NewKeyWithID("users", "u1")
	rec := record.NewRecordWithData(key, &User{Name: "Ada"}).SetError(nil)
	changes := &record.Changes{}
	changes.QueueForInsert(rec)
	changes.RecordsToUpdate = []*record.Updates{{
		Record:  rec,
		Updates: []update.Update{update.ByFieldName("name", "Grace")},
	}}

	write(t, db, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return dal.ApplyChanges(ctx, tx, changes)
	})

	got, err := dal.CollectionOf[string, User]().GetData(ctx, db, "u1")
	assert.NoError(t, err)
	assert.Equal(t, "Grace", got.Name)
	assert.Empty(t, changes.RecordsToInsert())
	assert.Empty(t, changes.RecordsToUpdate)
	assert.Empty(t, changes.RecordsToDelete)
}
