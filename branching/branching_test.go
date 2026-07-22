package branching_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/branching"
	"github.com/dal-go/dalgo/dal"
)

func TestDBInterfaceIsNotWidened(t *testing.T) {
	dbType := reflect.TypeOf((*dal.DB)(nil)).Elem()
	got := make([]string, dbType.NumMethod())
	for i := range dbType.NumMethod() {
		got[i] = dbType.Method(i).Name
	}
	want := []string{
		"Adapter",
		"ExecuteQueryToRecordsReader",
		"ExecuteQueryToRecordsetReader",
		"Exists",
		"Get",
		"GetMulti",
		"ID",
		"RunReadonlyTransaction",
		"RunReadwriteTransaction",
		"Schema",
		"SupportsConcurrentConnections",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dal.DB methods changed; branching must remain additive\ngot:  %v\nwant: %v", got, want)
	}
}

func TestUnsupportedError(t *testing.T) {
	err := &branching.UnsupportedError{Provider: "memory", Mode: "columnar", Reason: "not cloneable"}
	if !errors.Is(err, branching.ErrUnsupportedCapability) {
		t.Fatalf("errors.Is(%v, ErrUnsupportedCapability) = false", err)
	}
	if got := err.Error(); got == "" {
		t.Fatal("unsupported error is empty")
	}
}
