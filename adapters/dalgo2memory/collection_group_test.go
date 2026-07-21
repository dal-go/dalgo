package dalgo2memory

import (
	"context"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

type cgItem struct {
	Name string `json:"name"`
}

func cgKey(space, id string) *record.Key {
	return record.NewKeyWithParentAndID(record.NewKeyWithID("spaces", space), "items", id)
}

func cgSeed(t *testing.T, db dal.DB, space, id, name string) {
	t.Helper()
	rec := record.NewRecordWithData(cgKey(space, id), &cgItem{Name: name})
	require.NoError(t, db.RunReadwriteTransaction(context.Background(), func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, rec)
	}))
}

func cgQuery(collection string, parent *record.Key) dal.Query {
	ref := dal.NewRootCollectionRef(collection, "")
	if parent != nil {
		ref = dal.NewCollectionRef(collection, "", parent)
	}
	return dal.From(ref).NewQuery().SelectIntoRecord(func() record.Record {
		return record.NewRecordWithData(record.NewIncompleteKey(collection, reflect.String, nil), &cgItem{}).SetError(nil)
	})
}

// A collection-group query (root collection ref) returns records across every
// parent, each carrying its FULL parent chain — like Firestore.
func TestCollectionGroupPreservesParent(t *testing.T) {
	ctx := context.Background()
	db := NewDB()
	cgSeed(t, db, "spaceA", "i1", "a1")
	cgSeed(t, db, "spaceB", "i2", "b2")

	records, err := dal.ExecuteQueryAndReadAllToRecords(ctx, cgQuery("items", nil), db)
	require.NoError(t, err)
	require.Len(t, records, 2)

	bySpace := map[string]string{}
	for _, rec := range records {
		key := rec.Key()
		require.NotNil(t, key.Parent(), "result key must carry its parent (the space)")
		spaceID, _ := key.Parent().ID.(string)
		itemID, _ := key.ID.(string)
		bySpace[spaceID] = itemID
		// The materialized data comes back too.
		require.NotEmpty(t, rec.Data().(*cgItem).Name)
	}
	require.Equal(t, "i1", bySpace["spaceA"])
	require.Equal(t, "i2", bySpace["spaceB"])
}

// A parent-anchored collection ref scopes results to that parent's children.
func TestParentScopedQueryFiltersByParent(t *testing.T) {
	ctx := context.Background()
	db := NewDB()
	cgSeed(t, db, "spaceA", "i1", "a1")
	cgSeed(t, db, "spaceB", "i2", "b2")

	parentA := record.NewKeyWithID("spaces", "spaceA")
	records, err := dal.ExecuteQueryAndReadAllToRecords(ctx, cgQuery("items", parentA), db)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "i1", records[0].Key().ID)
	require.Equal(t, "spaceA", records[0].Key().Parent().ID)
}

func cgGet(t *testing.T, db dal.DB, space, id string) (*cgItem, error) {
	t.Helper()
	got := &cgItem{}
	rec := record.NewRecordWithData(cgKey(space, id), got)
	err := db.RunReadonlyTransaction(context.Background(), func(ctx context.Context, tx dal.ReadTransaction) error {
		return tx.Get(ctx, rec)
	})
	return got, err
}

func cgDelete(t *testing.T, db dal.DB, space, id string) {
	t.Helper()
	require.NoError(t, db.RunReadwriteTransaction(context.Background(), func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Delete(ctx, cgKey(space, id))
	}))
}

// Two records sharing a leaf id under different parents are distinct records —
// storing one must not overwrite the other, exactly as in Firestore where
// spaces/A/items/i1 and spaces/B/items/i1 are unrelated documents.
func TestSameLeafIdDifferentParentsDoNotCollide(t *testing.T) {
	ctx := context.Background()
	db := NewDB()
	cgSeed(t, db, "spaceA", "i1", "fromA")
	cgSeed(t, db, "spaceB", "i1", "fromB") // same leaf id "i1", different parent

	// Each parent keeps its own record (no silent overwrite).
	gotA, err := cgGet(t, db, "spaceA", "i1")
	require.NoError(t, err)
	require.Equal(t, "fromA", gotA.Name)
	gotB, err := cgGet(t, db, "spaceB", "i1")
	require.NoError(t, err)
	require.Equal(t, "fromB", gotB.Name)

	// A collection-group query returns both, each under its own parent.
	records, err := dal.ExecuteQueryAndReadAllToRecords(ctx, cgQuery("items", nil), db)
	require.NoError(t, err)
	require.Len(t, records, 2)
	byName := map[string]string{}
	for _, rec := range records {
		byName[rec.Data().(*cgItem).Name], _ = rec.Key().Parent().ID.(string)
	}
	require.Equal(t, "spaceA", byName["fromA"])
	require.Equal(t, "spaceB", byName["fromB"])

	// Deleting one parent's record leaves the other untouched.
	cgDelete(t, db, "spaceB", "i1")
	_, err = cgGet(t, db, "spaceA", "i1")
	require.NoError(t, err)
	_, err = cgGet(t, db, "spaceB", "i1")
	require.True(t, record.IsNotFound(err))
}

// The columnar engine must also keep same-leaf-id-across-parents distinct, and
// preserve each surviving record's full parent key across a compaction (which
// re-indexes slots).
func TestColumnarSameLeafIdSurvivesCompaction(t *testing.T) {
	ctx := context.Background()
	db := NewDB(WithSchema(false,
		WithCollection[cgItem]("items", nil, WithColumnarStorage()),
	))
	seed := func(space, id, name string) {
		require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			return tx.Set(ctx, record.NewRecordWithData(cgKey(space, id), &cgItem{Name: name}))
		}))
	}
	seed("spaceA", "i1", "a1")
	seed("spaceA", "i2", "a2")
	seed("spaceB", "i1", "b1") // same leaf id "i1" as spaceA — must not collide
	seed("spaceB", "i2", "b2")

	// Delete a majority to force compaction; the sole survivor is spaceA/i1.
	cgDelete(t, db, "spaceA", "i2")
	cgDelete(t, db, "spaceB", "i2")
	cgDelete(t, db, "spaceB", "i1")

	records, err := dal.ExecuteQueryAndReadAllToRecords(ctx, cgQuery("items", nil), db)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "i1", records[0].Key().ID)
	require.Equal(t, "spaceA", records[0].Key().Parent().ID, "parent key must survive compaction")
	require.Equal(t, "a1", records[0].Data().(*cgItem).Name)
}

func TestKeyIDRootAndNested(t *testing.T) {
	// Root-level key keeps the bare leaf id (backward compatible).
	require.Equal(t, "i1", keyID(record.NewKeyWithID("items", "i1")))
	// Nested key uses the full parent-chain path so it is globally unique.
	require.Equal(t, "spaces/spaceA/items/i1", keyID(cgKey("spaceA", "i1")))
}

func TestIsChildOf(t *testing.T) {
	space := record.NewKeyWithID("spaces", "s")
	child := record.NewKeyWithParentAndID(space, "items", "i1")
	require.False(t, isChildOf(nil, space))                                    // nil key
	require.False(t, isChildOf(child, nil))                                    // nil parent
	require.False(t, isChildOf(record.NewKeyWithID("items", "i1"), space))     // key has no parent
	require.False(t, isChildOf(child, record.NewKeyWithID("spaces", "other"))) // different parent
	require.True(t, isChildOf(child, space))                                   // match
}

func TestColumnarSlotKeyOutOfRange(t *testing.T) {
	e := &columnarEngine{}
	require.Nil(t, e.slotKey(0))
	require.Nil(t, e.slotKey(-1))
}

func TestNormalizeConstantMarshalError(t *testing.T) {
	ch := make(chan int) // channels are not JSON-marshalable
	require.Equal(t, ch, normalizeConstant(ch))
}
