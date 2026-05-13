package ddl

import (
	"fmt"
	"strings"
)

// AlterOp is the sealed interface for collection-altering operations.
// Defined here as a TEMPORARY type alias so PartialSuccessError can
// reference it in this task. Task 11 REPLACES this with the real
// sealed interface (`type AlterOp interface { alterOp() }`) plus six
// concrete constructors.
//
// NOTE TO IMPLEMENTER OF TASK 11: delete this `type AlterOp = ...`
// alias from this file; the real declaration lives in alter_op.go.
type AlterOp = interface{}

// PartialSuccessError is the typed error returned by AlterCollection
// (and any future batched DDL call) when a non-transactional driver
// succeeds at some sub-operations and fails at others.
//
// Distinct from *dbschema.NotSupportedError: NotSupportedError means
// "the driver can't do this at all"; PartialSuccessError means "the
// driver started doing this, then failed partway."
//
// Transactional drivers — those for which
// TransactionalDDL.SupportsTransactionalDDL returns true — MUST NOT
// produce *PartialSuccessError. Their failure mode is a regular
// error (rollback already performed; nothing was applied).
//
// Unwrap returns Cause so errors.Is(err, dal.ErrNotSupported)
// propagates transitively if the underlying failure was a
// not-supported case.
type PartialSuccessError struct {
	// Op names the batched operation (currently always "AlterCollection").
	Op string
	// Collection is the target collection name.
	Collection string
	// Backend optionally identifies the driver.
	Backend string
	// Applied lists the ops that completed successfully, in original
	// order.
	Applied []AlterOp
	// FirstFailed is the op that failed.
	FirstFailed AlterOp
	// NotAttempted lists the ops that came after FirstFailed and were
	// not tried. May be empty if the driver attempted every op
	// regardless of earlier failures.
	NotAttempted []AlterOp
	// Cause is the underlying error from the failed op (driver-specific).
	Cause error
}

// Error returns a readable single-line summary.
func (e *PartialSuccessError) Error() string {
	var b strings.Builder
	b.WriteString("ddl: partial success: ")
	if e.Op != "" {
		fmt.Fprintf(&b, "op=%s ", e.Op)
	}
	if e.Collection != "" {
		fmt.Fprintf(&b, "collection=%s ", e.Collection)
	}
	if e.Backend != "" {
		fmt.Fprintf(&b, "backend=%s ", e.Backend)
	}
	fmt.Fprintf(&b, "applied=%d failed=1 not_attempted=%d", len(e.Applied), len(e.NotAttempted))
	if e.Cause != nil {
		fmt.Fprintf(&b, "; cause: %v", e.Cause)
	}
	return b.String()
}

// Unwrap returns Cause so errors.Is and errors.As propagate
// through the underlying driver-specific failure.
func (e *PartialSuccessError) Unwrap() error {
	return e.Cause
}
