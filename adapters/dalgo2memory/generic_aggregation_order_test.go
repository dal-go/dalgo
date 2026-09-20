package dalgo2memory

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/require"
)

func TestDALgoGenericOrderByUnselectedAggregate(t *testing.T) {
	backend, ctx := seedSales(t)
	count := dal.Count()
	q := salesQuery().
		GroupBy(dal.Field("category")).
		OrderBy(dal.Descending(count.Expression)).
		SelectColumns(dal.Column{Expression: dal.Field("category")})
	reader, err := dal.NewDB(hashOnlyBackend{Backend: backend}).ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	rows := readResultMaps(t, reader)
	require.Equal(t, []map[string]any{{"category": "A"}, {"category": "B"}}, rows)
}
