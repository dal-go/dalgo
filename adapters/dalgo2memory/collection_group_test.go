package dalgo2memory

import (
	"context"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/require"
)

type cgItem struct {
	Name string `json:"name"`
}

func cgKey(space, id string) *dal.Key {
	return dal.NewKeyWithParentAndID(dal.NewKeyWithID("spaces", space), "items", id)
}

func cgSeed(t *testing.T, db dal.DB, space, id, name string) {
	t.Helper()
	rec := dal.NewRecordWithData(cgKey(space, id), &cgItem{Name: name})
	require.NoError(t, db.RunReadwriteTransaction(context.Background(), func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, rec)
	}))
}

func cgQuery(collection string, parent *dal.Key) dal.Query {
	ref := dal.NewRootCollectionRef(collection, "")
	if parent != nil {
		ref = dal.NewCollectionRef(collection, "", parent)
	}
	return dal.From(ref).NewQuery().SelectIntoRecord(func() dal.Record {
		return dal.NewRecordWithData(dal.NewIncompleteKey(collection, reflect.String, nil), &cgItem{}).SetError(nil)
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

	parentA := dal.NewKeyWithID("spaces", "spaceA")
	records, err := dal.ExecuteQueryAndReadAllToRecords(ctx, cgQuery("items", parentA), db)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "i1", records[0].Key().ID)
	require.Equal(t, "spaceA", records[0].Key().Parent().ID)
}

func TestIsChildOf(t *testing.T) {
	space := dal.NewKeyWithID("spaces", "s")
	child := dal.NewKeyWithParentAndID(space, "items", "i1")
	require.False(t, isChildOf(nil, space))                                 // nil key
	require.False(t, isChildOf(child, nil))                                 // nil parent
	require.False(t, isChildOf(dal.NewKeyWithID("items", "i1"), space))     // key has no parent
	require.False(t, isChildOf(child, dal.NewKeyWithID("spaces", "other"))) // different parent
	require.True(t, isChildOf(child, space))                                // match
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
