package ddl

import "github.com/dal-go/dalgo/dal"

// TransactionalDDL is the optional capability interface drivers
// implement to advertise that they guarantee all-or-nothing
// atomicity for DDL calls that perform multiple sub-operations
// (notably AlterCollection with multiple AlterOps, or
// CreateCollection with inline indexes on some engines).
//
// The pattern mirrors [dal.ConcurrencyAware]: a one-method optional
// interface that consumers type-assert against. Drivers that don't
// implement TransactionalDDL are treated as non-transactional.
//
// When SupportsTransactionalDDL returns true: if any sub-operation
// fails, the driver MUST roll back all previously-applied
// sub-operations in the same call. The whole call returns a non-nil
// error; the DB is left in its pre-call state.
//
// When SupportsTransactionalDDL returns false (or the interface is
// not implemented): the driver MAY apply some sub-operations and
// fail on others. Callers receive a *PartialSuccessError listing
// applied / failed / not-attempted ops.
//
// The return value is constant from the moment a DB value is
// returned by its constructor until it is discarded; the same
// stability contract as dal.ConcurrencyAware.
type TransactionalDDL interface {
	SupportsTransactionalDDL() bool
}

// SupportsTransactionalDDL is the convenience helper that
// encapsulates the type assertion and the convention that "doesn't
// implement = treat as non-transactional." Consumers SHOULD use this
// rather than performing the assertion themselves.
func SupportsTransactionalDDL(db dal.DB) bool {
	a, ok := db.(TransactionalDDL)
	if !ok {
		return false
	}
	return a.SupportsTransactionalDDL()
}
