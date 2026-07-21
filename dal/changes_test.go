package dal_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/mocks/mock_dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
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
	different := record.NewRecordWithData(record.NewKeyWithID("users", "u2"), &User{Name: "Lin"})
	changes := new(dal.Changes)

	assert.False(t, changes.IsChanged(nil))
	assert.False(t, changes.IsChanged(first))
	changes.FlagAsChanged(first)
	assert.True(t, changes.IsChanged(first))
	changes.FlagAsChanged(duplicate)

	assert.True(t, first.HasChanged())
	assert.True(t, duplicate.HasChanged())
	assert.True(t, changes.IsChanged(duplicate))
	assert.False(t, changes.IsChanged(different))
	assert.True(t, changes.HasChanges())
	records := changes.Records()
	assert.Len(t, records, 1)
	records[0] = nil
	assert.NotNil(t, changes.Records()[0], "Records must return a copy")
}

func TestChangesRejectsNilRecord(t *testing.T) {
	assert.Panics(t, func() { new(dal.Changes).FlagAsChanged(nil) })
}

func TestApplyChangesRejectsNilChangeSet(t *testing.T) {
	assert.PanicsWithValue(t, "changes == nil", func() {
		_ = dal.ApplyChanges(context.Background(), nil, nil)
	})
}

func TestApplyChangesExcludesInsertsAndDeletes(t *testing.T) {
	ctx := context.Background()
	tx := mock_dal.NewMockReadwriteTransaction(gomock.NewController(t))
	excludedKey := record.NewKeyWithID("users", "excluded")
	included := record.NewRecordWithData(record.NewKeyWithID("users", "included"), &User{Name: "Ada"}).SetError(nil)
	deletedKey := record.NewKeyWithID("users", "deleted")
	changes := &record.Changes{RecordsToDelete: []*record.Key{deletedKey}}
	changes.QueueForInsert(record.NewRecordWithData(excludedKey, &User{Name: "Grace"}).SetError(nil), included)

	tx.EXPECT().InsertMulti(ctx, []record.Record{included}).Return(nil)
	tx.EXPECT().DeleteMulti(ctx, []*record.Key{deletedKey}).Return(nil)

	assert.NoError(t, dal.ApplyChanges(ctx, tx, changes, excludedKey))
	assert.Empty(t, changes.RecordsToInsert())
	assert.Empty(t, changes.RecordsToDelete)
}

func TestApplyChangesPreservesChangeSetOnFailure(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("write failed")

	t.Run("insert", func(t *testing.T) {
		tx := mock_dal.NewMockReadwriteTransaction(gomock.NewController(t))
		rec := record.NewRecordWithData(record.NewKeyWithID("users", "insert"), &User{Name: "Ada"}).SetError(nil)
		changes := &record.Changes{}
		changes.QueueForInsert(rec)
		tx.EXPECT().InsertMulti(ctx, []record.Record{rec}).Return(wantErr)

		err := dal.ApplyChanges(ctx, tx, changes)
		assert.ErrorIs(t, err, wantErr)
		assert.ErrorContains(t, err, "failed to insert records")
		assert.Len(t, changes.RecordsToInsert(), 1)
	})

	t.Run("update", func(t *testing.T) {
		tx := mock_dal.NewMockReadwriteTransaction(gomock.NewController(t))
		rec := record.NewRecordWithData(record.NewKeyWithID("users", "update"), &User{Name: "Ada"})
		updates := []update.Update{update.ByFieldName("name", "Grace")}
		changes := &record.Changes{RecordsToUpdate: []*record.Updates{{Record: rec, Updates: updates}}}
		tx.EXPECT().Update(ctx, rec.Key(), updates).Return(wantErr)

		err := dal.ApplyChanges(ctx, tx, changes)
		assert.ErrorIs(t, err, wantErr)
		assert.ErrorContains(t, err, "failed to update record")
		assert.Len(t, changes.RecordsToUpdate, 1)
	})

	t.Run("delete", func(t *testing.T) {
		tx := mock_dal.NewMockReadwriteTransaction(gomock.NewController(t))
		key := record.NewKeyWithID("users", "delete")
		changes := &record.Changes{RecordsToDelete: []*record.Key{key}}
		tx.EXPECT().DeleteMulti(ctx, []*record.Key{key}).Return(wantErr)

		err := dal.ApplyChanges(ctx, tx, changes)
		assert.ErrorIs(t, err, wantErr)
		assert.ErrorContains(t, err, "failed to delete records")
		assert.Len(t, changes.RecordsToDelete, 1)
	})
}
