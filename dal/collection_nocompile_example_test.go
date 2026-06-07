//go:build dalgo_collection_nocompile

// This file is the NEGATIVE-COMPILE proof for AC write-needs-write-session: a
// write terminal MUST NOT accept a plain dal.DB (which satisfies ReadSession
// but not WriteSession). It is excluded from normal builds and CI by the
// build tag above.
//
// To verify it fails to compile, run (go vet/test compile _test.go files,
// whereas go build skips them):
//
//	go vet -tags dalgo_collection_nocompile ./dal/
//
// which MUST fail with a type error on the Set call below (dal.DB does not
// implement dal.WriteSession). Removing the call would make this file compile,
// which is exactly what the AC forbids.
package dal_test

import (
	"context"

	"github.com/dal-go/dalgo/dal"
)

func writeTerminalRejectsPlainDB(ctx context.Context, db dal.DB) {
	users := dal.CollectionOf[User]()
	// COMPILE ERROR (expected): dal.DB does not satisfy dal.WriteSession.
	_ = users.Set(ctx, db, "u1", User{Name: "Alice"})
}

// generatorRejectedByIDTakingTerminals is the NEGATIVE-COMPILE proof for AC
// generator-not-accepted-elsewhere: only the bare Insert accepts
// ...dal.InsertOption; the id-taking terminals (InsertWithID/Get/Set/Update/
// Delete) MUST NOT, so passing a generator to them is a compile error.
func generatorRejectedByIDTakingTerminals(ctx context.Context, tx dal.ReadwriteTransaction) {
	users := dal.CollectionOf[User]()
	gen := dal.WithRandomStringKey(16, 5)

	// Each line below is an expected COMPILE ERROR: these terminals take no
	// InsertOption.
	_, _ = users.InsertWithID(ctx, tx, "u1", User{}, gen)
	_, _ = users.Get(ctx, tx, "u1", gen)
	_ = users.Set(ctx, tx, "u1", User{}, gen)
	_ = users.Update(ctx, tx, "u1", nil, gen)
	_ = users.Delete(ctx, tx, "u1", gen)

	// The bare Insert, by contrast, DOES accept it (this line is valid).
	_, _ = users.Insert(ctx, tx, User{}, gen)
}
