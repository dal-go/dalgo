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

func TestChangesTracksUniqueRecordKeys(t *testing.T) {
	key := record.NewKeyWithID("users", "u1")
	first := record.NewRecordWithData(key, &User{Name: "Ada"})
	duplicate := record.NewRecordWithData(record.NewKeyWithID("users", "u1"), &User{Name: "Grace"})
	changes := new(dal.Changes)

	changes.FlagAsChanged(first)
	changes.FlagAsChanged(duplicate)

	assert.True(t, first.HasChanged())
	assert.True(t, duplicate.HasChanged())
	assert.True(t, changes.IsChanged(duplicate))
	assert.True(t, changes.HasChanges())
	records := changes.Records()
	assert.Len(t, records, 1)
	records[0] = nil
	assert.NotNil(t, changes.Records()[0], "Records must return a copy")
}

func TestChangesRejectsNilRecord(t *testing.T) {
	assert.Panics(t, func() { new(dal.Changes).FlagAsChanged(nil) })
}
