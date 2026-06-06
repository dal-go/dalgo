package dalgo2memory_test

import (
	"context"
	"reflect"
	"testing"

	dalgo2memory2 "github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/require"
)

// externalUser is a typed record defined in the EXTERNAL test package, so this
// test exercises only dalgo2memory's exported surface.
type externalUser struct {
	Name string
	Role string
}

// externalStrategy is a ColumnStrategy implemented entirely outside the
// dalgo2memory package, using only its exported ColumnStrategy interface and
// SlotSet type. It models what an out-of-core package (e.g. a bitmap index)
// would provide: an index kept in sync via the write side that answers equality
// by returning matching slots. This proves dalgo2memory does not need to import
// the strategy's package (REQ:external-strategy-pluggable).
type externalStrategy struct {
	bySlot map[int]any
	calls  int
}

func newExternalStrategy() *externalStrategy {
	return &externalStrategy{bySlot: map[int]any{}}
}

func (s *externalStrategy) SetValue(slot int, value any) { s.bySlot[slot] = value }

func (s *externalStrategy) ClearValue(slot int) { delete(s.bySlot, slot) }

func (s *externalStrategy) EqualSlots(value any) (dalgo2memory2.SlotSet, bool) {
	s.calls++
	slots := make(dalgo2memory2.SlotSet)
	for slot, v := range s.bySlot {
		if v == value {
			slots[slot] = struct{}{}
		}
	}
	return slots, true
}

// WithExternalRoleIndex mirrors a hypothetical bitmap4dalgo2memory.WithBitmapColumn:
// an exported constructor returning a ColumnOption that plugs an external
// strategy into WithColumnarStorage.
func WithExternalRoleIndex(s *externalStrategy) dalgo2memory2.ColumnOption {
	return dalgo2memory2.WithColumnStrategy("Role", s)
}

// TestExternalStrategyPluggable verifies
// columnar-storage#ac:strategy-interface-exported: an external strategy supplied
// via WithColumnarStorage is consulted by the query engine, with results
// identical to a Serialized collection — and dalgo2memory never imports it.
func TestExternalStrategyPluggable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	stgy := newExternalStrategy()
	colDB := dalgo2memory2.NewDB(dalgo2memory2.WithSchema(false,
		dalgo2memory2.WithCollection[externalUser]("users", nil,
			dalgo2memory2.WithColumnarStorage(WithExternalRoleIndex(stgy))),
	))
	serDB := dalgo2memory2.NewDB(dalgo2memory2.WithSchema(false,
		dalgo2memory2.WithCollection[externalUser]("users", nil),
	))

	seed := func(db dal.DB) {
		err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			records := []dal.Record{
				dal.NewRecordWithData(dal.NewKeyWithID("users", "u1"), &externalUser{Name: "Alice", Role: "admin"}),
				dal.NewRecordWithData(dal.NewKeyWithID("users", "u2"), &externalUser{Name: "Bob", Role: "member"}),
				dal.NewRecordWithData(dal.NewKeyWithID("users", "u3"), &externalUser{Name: "Carol", Role: "admin"}),
			}
			return tx.SetMulti(ctx, records)
		})
		require.NoError(t, err)
	}
	seed(colDB)
	seed(serDB)

	build := func() dal.Query {
		return dal.From(dal.NewRootCollectionRef("users", "")).NewQuery().
			WhereField("Role", dal.Equal, "admin").
			SelectKeysOnly(reflect.String)
	}
	colReader, err := colDB.ExecuteQueryToRecordsReader(ctx, build())
	require.NoError(t, err)
	serReader, err := serDB.ExecuteQueryToRecordsReader(ctx, build())
	require.NoError(t, err)

	require.Positive(t, stgy.calls, "the external strategy's read side was consulted")
	require.Equal(t, idsOf(t, serReader), idsOf(t, colReader))
}

func idsOf(t *testing.T, reader dal.RecordsReader) []string {
	t.Helper()
	var ids []string
	for {
		record, err := reader.Next()
		if err != nil {
			require.ErrorIs(t, err, dal.ErrNoMoreRecords)
			return ids
		}
		ids = append(ids, record.Key().ID.(string))
	}
}
