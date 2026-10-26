package dalgo2memory

import (
	"context"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
)

func TestCollectionGroupDocumentIDUsesFullIdentity(t *testing.T) {
	for _, pointer := range []bool{false, true} {
		t.Run(map[bool]string{false: "value", true: "pointer"}[pointer], func(t *testing.T) {
			db := newDatabase()
			ctx := context.Background()
			keys := []*record.Key{record.NewKeyWithID("items", "root")}
			for _, item := range []struct{ parent, id string }{{"a", "z"}, {"b", "a"}, {"c", "same"}, {"d", "same"}} {
				keys = append(keys, record.NewKeyWithParentAndID(record.NewKeyWithID("parents", item.parent), "items", item.id))
			}
			for _, key := range keys {
				require.NoError(t, db.Set(ctx, record.NewRecordWithData(key, &orderThing{Name: key.String()})))
			}
			ref := dal.NewCollectionGroupRef("items", "")
			var source dal.RecordsetSource = ref
			if pointer {
				source = &ref
			}
			query := func(desc bool, from, after string, offset, limit int) dal.StructuredQuery {
				order := dal.Ascending(dal.DocumentID())
				if desc {
					order = dal.Descending(dal.DocumentID())
				}
				b := dal.From(source).NewQuery().OrderBy(order).Offset(offset).Limit(limit)
				if from != "" {
					b = b.StartFrom(dal.Cursor(from))
				}
				if after != "" {
					b = b.StartAfter(dal.Cursor(after))
				}
				return b.SelectIntoRecord(func() record.Record {
					return record.NewRecordWithIncompleteKey("items", reflect.String, new(orderThing))
				})
			}
			all := []string{"items/root", "parents/a/items/z", "parents/b/items/a", "parents/c/items/same", "parents/d/items/same"}
			require.Equal(t, all, runSingleSource(t, db, ctx, query(false, "", "", 0, 0)))
			require.Equal(t, all[2:], runSingleSource(t, db, ctx, query(false, all[2], "", 0, 0)))
			require.Equal(t, all[3:], runSingleSource(t, db, ctx, query(false, "", all[2], 0, 0)))
			require.Equal(t, []string{all[2], all[1], all[0]}, runSingleSource(t, db, ctx, query(true, all[2], "", 0, 0)))
			require.Equal(t, []string{all[1], all[0]}, runSingleSource(t, db, ctx, query(true, "", all[2], 0, 0)))
			require.Equal(t, all[1:3], runSingleSource(t, db, ctx, query(false, "", "", 1, 2)))
			require.Equal(t, all[3:], runSingleSource(t, db, ctx, query(false, "", "parents/bb/items/deleted", 0, 0)))
			var paged []string
			cursor := ""
			for range 6 {
				page := runSingleSource(t, db, ctx, query(false, "", cursor, 0, 1))
				if len(page) == 0 {
					break
				}
				paged = append(paged, page...)
				cursor = page[len(page)-1]
			}
			require.Equal(t, all, paged)
			require.NoError(t, db.Delete(ctx, keys[2]))
			require.Equal(t, all[3:], runSingleSource(t, db, ctx, query(false, "", all[2], 0, 0)))
			require.Equal(t, all[3:], runSingleSource(t, db, ctx, query(false, all[2], "", 0, 0)))
			require.Equal(t, []string{all[1], all[0]}, runSingleSource(t, db, ctx, query(true, all[2], "", 0, 0)))
			require.Equal(t, []string{all[1], all[0]}, runSingleSource(t, db, ctx, query(true, "", all[2], 0, 0)))
		})
	}
}

func TestCollectionGroupDuplicateLeafIDPagination(t *testing.T) {
	db := newDatabase()
	ctx := context.Background()
	var want []string
	for _, parent := range []string{"a", "b"} {
		key := record.NewKeyWithParentAndID(record.NewKeyWithID("parents", parent), "items", "same")
		want = append(want, key.String())
		require.NoError(t, db.Set(ctx, record.NewRecordWithData(key, &orderThing{Name: key.String()})))
	}
	var got []string
	cursor := ""
	for range 3 {
		b := dal.From(dal.NewCollectionGroupRef("items", "")).NewQuery().OrderBy(dal.Ascending(dal.DocumentID())).Limit(1)
		if cursor != "" {
			b = b.StartAfter(dal.Cursor(cursor))
		}
		q := b.SelectIntoRecord(func() record.Record {
			return record.NewRecordWithIncompleteKey("items", reflect.String, new(orderThing))
		})
		page := runSingleSource(t, db, ctx, q)
		if len(page) == 0 {
			break
		}
		got = append(got, page...)
		cursor = page[0]
	}
	require.Equal(t, want, got, "each full document identity must appear once, even when leaf IDs repeat")
}
