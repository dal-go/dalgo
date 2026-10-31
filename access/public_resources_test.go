package access

import (
	"context"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func TestResourcesForQueryInspectionMatchesExecution(t *testing.T) {
	ref := dal.NewRootCollectionRef("Customer", "c")
	cyclic := &shapeQuery{}
	cyclic.from = dal.From(dal.NewQuerySource(cyclic, "loop"))
	for _, query := range []dal.Query{dal.From(ref).NewQuery().SelectKeysOnly(reflect.String), dal.NewTextQuery("SELECT 1", nil), &shapeQuery{}, cyclic, make(inspectionChannelQuery)} {
		got := ResourcesForQuery(query)
		want := resourcesForQuery(query)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("inspection diverged from execution: %v vs %v", got, want)
		}
	}
	var typedNil *shapeQuery
	for _, query := range []dal.Query{nil, typedNil, inspectionChannelQuery(nil)} {
		got := ResourcesForQuery(query)
		if len(got) != 1 || got[0].kind != OpaqueQueryResource {
			t.Fatalf("nil query must be opaque: %v", got)
		}
	}
}

type inspectionChannelQuery chan struct{}

func (q inspectionChannelQuery) String() string {
	if q == nil {
		panic("nil channel String must not run")
	}
	return "channel"
}
func (inspectionChannelQuery) Offset() int { return 0 }
func (inspectionChannelQuery) Limit() int  { return 0 }
func (inspectionChannelQuery) GetRecordsReader(context.Context, dal.QueryExecutor) (dal.RecordsReader, error) {
	return nil, nil
}
func (inspectionChannelQuery) GetRecordsetReader(context.Context, dal.QueryExecutor) (dal.RecordsetReader, error) {
	return nil, nil
}
