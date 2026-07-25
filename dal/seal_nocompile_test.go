//go:build dalgo_seal_nocompile

// This file is the NEGATIVE-COMPILE proof for layer 2 of framework-enforced
// record invariants: a type that implements every dal.DB method but does NOT
// embed dal.DB must fail to satisfy dal.DB, so an adapter cannot hand a caller
// a raw backend that skips the write pipeline.
//
// It is excluded from normal builds by the tag above, and
// TestSealIsEnforcedAtCompileTime runs
//
//	go vet -tags dalgo_seal_nocompile .
//
// and requires it to FAIL. If that command ever succeeds, the seal is gone.
package dal_test

import (
	"context"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
)

// unsealedDB implements the whole dal.Backend surface — and nothing else.
type unsealedDB struct {
	dal.NoConcurrency
}

func (unsealedDB) ID() string           { return "unsealed" }
func (unsealedDB) Adapter() dal.Adapter { return nil }
func (unsealedDB) Schema() dal.Schema   { return nil }
func (unsealedDB) RunReadonlyTransaction(context.Context, dal.ROTxWorker, ...dal.TransactionOption) error {
	return nil
}
func (unsealedDB) RunReadwriteTransaction(context.Context, dal.RWTxWorker, ...dal.TransactionOption) error {
	return nil
}
func (unsealedDB) Get(context.Context, record.Record) error { return nil }
func (unsealedDB) Exists(context.Context, *record.Key) (bool, error) {
	return false, nil
}
func (unsealedDB) GetMulti(context.Context, []record.Record) error { return nil }
func (unsealedDB) ExecuteQueryToRecordsReader(context.Context, dal.Query) (dal.RecordsReader, error) {
	return nil, nil
}
func (unsealedDB) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, nil
}

// It satisfies dal.Backend — that is the adapter contract and it is met.
var _ dal.Backend = unsealedDB{}

// COMPILE ERROR (expected): unsealedDB does not implement dal.DB, because the
// seal method is unexported and it embeds no dal.DB.
var _ dal.DB = unsealedDB{}
