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

// TestValidatedTxHasStableComparableIdentity covers adapter transactions that
// are values containing slices, maps, functions, or similarly non-comparable
// state. Interface equality on the framework wrapper must never recurse into
// that state and panic.
func TestValidatedTxHasStableComparableIdentity(t *testing.T) {
	underlying := nonComparableIDOnlyTx{id: "tx-non-comparable", marker: []byte{1}}
	left := ReadwriteTransaction(newValidatedTx(underlying, nil))
	same := left
	if left != same {
		t.Fatal("the same validated transaction wrapper has different identity")
	}

	right := ReadwriteTransaction(newValidatedTx(underlying, nil))
	if left == right {
		t.Fatal("separately created validated transaction wrappers share identity")
	}
	if got := left.ID(); got != underlying.id {
		t.Fatalf("validated transaction ID = %q, want %q", got, underlying.id)
	}
}

func TestValidatedTxWithoutValidationRemainsPointerBacked(t *testing.T) {
	tx := newValidatedTx(idOnlyTx{id: "tx-without-validation"}, nil)
	unvalidated, ok := tx.dalgoWithoutValidation().(*validatedTx)
	if !ok {
		t.Fatalf("WithoutValidation transaction = %T, want *validatedTx", unvalidated)
	}
	if unvalidated.validate {
		t.Fatal("WithoutValidation transaction still validates writes")
	}
	if !tx.validate {
		t.Fatal("WithoutValidation mutated the original transaction wrapper")
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

type nonComparableIDOnlyTx struct {
	ReadwriteTransaction
	id     string
	marker []byte
}

func (tx nonComparableIDOnlyTx) ID() string { return tx.id }
