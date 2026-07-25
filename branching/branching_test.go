package branching_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/branching"
	"github.com/dal-go/dalgo/dal"
)

// TestDBInterfaceIsNotWidened guards that the branching capability stays
// additive: it must not add methods to the interface storage adapters
// implement. That interface is dal.Backend — dal.DB is the same shape plus the
// unexported seal, asserted separately below.
func TestDBInterfaceIsNotWidened(t *testing.T) {
	backendType := reflect.TypeOf((*dal.Backend)(nil)).Elem()
	got := make([]string, backendType.NumMethod())
	for i := range backendType.NumMethod() {
		got[i] = backendType.Method(i).Name
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
		t.Fatalf("dal.Backend methods changed; branching must remain additive\ngot:  %v\nwant: %v", got, want)
	}

	// dal.DB is dal.Backend plus exactly one unexported seal method. Anything
	// else means the caller-facing surface drifted from the adapter contract.
	dbType := reflect.TypeOf((*dal.DB)(nil)).Elem()
	if dbType.NumMethod() != backendType.NumMethod()+1 {
		t.Fatalf("dal.DB has %d methods, want dal.Backend's %d plus the seal",
			dbType.NumMethod(), backendType.NumMethod())
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

func TestUnsupportedErrorWithoutReason(t *testing.T) {
	err := &branching.UnsupportedError{Provider: "memory", Mode: "custom"}
	if got, want := err.Error(), `dalgo branching: unsupported capability: provider="memory" mode="custom"`; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
