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
