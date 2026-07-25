package dal

import (
	"context"
	"testing"
)

// TestValidatedTxForwardsTransactionIdentity: a caller inside a read-write
// transaction must still see the adapter's transaction id, or correlating logs
// with backend traces stops working.
func TestValidatedTxForwardsTransactionIdentity(t *testing.T) {
	tx := newValidatedTx(idOnlyTx{id: "tx-42"}, nil)
	if got := tx.ID(); got != "tx-42" {
		t.Fatalf("validatedTx.ID() = %q, want the adapter transaction's id", got)
	}
}

// TestRecordDataToValidateHandlesANilRecord: BeforeSave is exported, so a caller
// can reach it with a nil record. It must not panic there.
func TestRecordDataToValidateHandlesANilRecord(t *testing.T) {
	if got := recordDataToValidate(nil); got != nil {
		t.Fatalf("recordDataToValidate(nil) = %v, want nil", got)
	}
	if err := BeforeSave(context.Background(), nil, nil); err != nil {
		t.Fatalf("BeforeSave with a nil record: err = %v, want nil", err)
	}
}

// idOnlyTx is a read-write transaction that only knows its id; the pipeline
// never touches the rest in this test.
type idOnlyTx struct {
	ReadwriteTransaction
	id string
}

func (tx idOnlyTx) ID() string { return tx.id }
