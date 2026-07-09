package dalgo2memory

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/update"
	"github.com/stretchr/testify/require"
)

// TestFieldPathUpdate_SetsNestedValue verifies that an update created via
// update.ByFieldPath sets a nested value inside a stored map[string]any record.
func TestFieldPathUpdate_SetsNestedValue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("docs", "d1")

	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, map[string]any{
		"meta": map[string]any{"version": 1},
	})))

	require.NoError(t, db.Update(ctx, key, []update.Update{
		update.ByFieldPath(update.FieldPath{"meta", "version"}, 2),
	}))

	var got map[string]any
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	meta, ok := got["meta"].(map[string]any)
	require.True(t, ok, "meta should still be a map")
	require.EqualValues(t, 2, meta["version"], "nested value should be updated to 2")
}

// TestFieldPathUpdate_DeleteFieldByFieldName verifies that update.DeleteField
// used with update.ByFieldName removes the top-level key from the stored record.
func TestFieldPathUpdate_DeleteFieldByFieldName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("docs", "d1")

	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, map[string]any{
		"keep": "yes",
		"drop": "goodbye",
	})))

	require.NoError(t, db.Update(ctx, key, []update.Update{
		update.DeleteByFieldName("drop"),
	}))

	var got map[string]any
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	require.Equal(t, "yes", got["keep"], "unaffected key must survive")
	_, hasDropped := got["drop"]
	require.False(t, hasDropped, "deleted key must not be present")
}

// TestFieldPathUpdate_DeleteFieldByFieldPath removes a nested map entry while
// leaving sibling entries intact. This mirrors the contactus brief-map scenario:
// a top-level field holds a map[string]any, one entry is deleted by FieldPath,
// and the other entry survives.
func TestFieldPathUpdate_DeleteFieldByFieldPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("docs", "d1")

	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, map[string]any{
		"members": map[string]any{
			"alice": true,
			"bob":   true,
		},
	})))

	require.NoError(t, db.Update(ctx, key, []update.Update{
		update.ByFieldPath(update.FieldPath{"members", "alice"}, update.DeleteField),
	}))

	var got map[string]any
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	members, ok := got["members"].(map[string]any)
	require.True(t, ok, "members should still be a map")
	_, alicePresent := members["alice"]
	require.False(t, alicePresent, "alice should have been deleted")
	require.NotNil(t, members["bob"], "bob should survive the deletion")
}

// TestFieldPathUpdate_NonMapIntermediateReturnsError verifies that when a
// FieldPath update encounters an intermediate node that is not a map[string]any,
// the adapter returns a descriptive error and does not panic.
func TestFieldPathUpdate_NonMapIntermediateReturnsError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("docs", "d1")

	// "meta" is a string, not a map — navigating through it must fail gracefully.
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, map[string]any{
		"meta": "just a string",
	})))

	err := db.Update(ctx, key, []update.Update{
		update.ByFieldPath(update.FieldPath{"meta", "version"}, 2),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "meta")
}

// TestFieldPathUpdate_PlainFieldNameStillWorks is a regression test: plain
// ByFieldName updates must continue to work exactly as before.
func TestFieldPathUpdate_PlainFieldNameStillWorks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("docs", "d1")

	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, map[string]any{
		"title": "original",
		"count": 1,
	})))

	require.NoError(t, db.Update(ctx, key, []update.Update{
		update.ByFieldName("title", "updated"),
	}))

	var got map[string]any
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	require.Equal(t, "updated", got["title"])
	require.EqualValues(t, 1, got["count"], "untouched field must survive")
}

// TestFieldPathUpdate_CreatesMissingIntermediates verifies that a FieldPath
// update creates missing intermediate maps on its way to the leaf.
func TestFieldPathUpdate_CreatesMissingIntermediates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("docs", "d5")

	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, map[string]any{"title": "x"})))

	require.NoError(t, db.Update(ctx, key, []update.Update{
		update.ByFieldPath(update.FieldPath{"meta", "nested", "flag"}, true),
	}))

	var got map[string]any
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	meta, ok := got["meta"].(map[string]any)
	require.True(t, ok, "meta should be created as a map")
	nested, ok := meta["nested"].(map[string]any)
	require.True(t, ok, "nested should be created as a map")
	require.Equal(t, true, nested["flag"])
}

// emptyUpdate is an update.Update with neither FieldName nor FieldPath set —
// applyUpdatesToMap must skip it without error.
type emptyUpdate struct{}

func (emptyUpdate) FieldName() string           { return "" }
func (emptyUpdate) FieldPath() update.FieldPath { return nil }
func (emptyUpdate) Value() any                  { return nil }

// TestApplyUpdatesToMap_SkipsUpdateWithoutNameOrPath verifies the defensive
// skip branch for updates carrying neither a field name nor a field path.
func TestApplyUpdatesToMap_SkipsUpdateWithoutNameOrPath(t *testing.T) {
	t.Parallel()
	data := map[string]any{"a": 1}
	require.NoError(t, applyUpdatesToMap(data, []update.Update{emptyUpdate{}}))
	require.Equal(t, map[string]any{"a": 1}, data, "data must be unchanged")
}

// TestColumnar_FieldPathUpdateThroughNonMapFails verifies that the columnar
// engine surfaces applyUpdatesToMap errors when a FieldPath update walks
// through a non-map value.
func TestColumnar_FieldPathUpdateThroughNonMapFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := columnarItemDB(t)
	key := dal.NewKeyWithID("items", "i9")
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, &item{Name: "a", Count: 1})))

	err := db.Update(ctx, key, []update.Update{
		update.ByFieldPath(update.FieldPath{"Name", "sub"}, 1),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "is not a map")
}
