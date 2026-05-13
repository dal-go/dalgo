package ddl

import (
	"context"

	"github.com/dal-go/dalgo/dbschema"
)

// SchemaModifier is the capability interface drivers implement to
// support DDL operations on dalgo collections. The interface is
// three methods:
//
//   - CreateCollection creates a collection (table) along with any
//     inline indexes declared in CollectionDef.Indexes.
//   - DropCollection drops a collection along with its indexes.
//   - AlterCollection applies a batch of AlterOp values
//     (field-level and index-level alterations) to an existing
//     collection. The driver decides how to apply the batch
//     atomically; consumers check [TransactionalDDL] to know
//     whether to expect rollback.
//
// SchemaModifier is NOT embedded into [dal.DB]. DDL is genuinely
// optional for some backends — read-only wrappers, analytics
// drivers, mocks. Drivers opt in by implementing SchemaModifier on
// their dal.DB value (or a related type reachable via type
// assertion). The top-level helper functions [CreateCollection],
// [DropCollection], and [AlterCollection] type-assert against
// SchemaModifier and return *dbschema.NotSupportedError on the
// failed assertion.
//
// Index-level operations after initial collection creation are NOT
// separate methods — they are AlterOp values passed to
// AlterCollection. See [AddIndex] and [DropIndex].
type SchemaModifier interface {
	// CreateCollection creates the collection (table) and any inline
	// indexes declared on c.Indexes. Caller passes opts for opt-in
	// idempotency.
	CreateCollection(ctx context.Context, c dbschema.CollectionDef, opts ...Option) error

	// DropCollection drops the collection and its indexes. Caller
	// passes opts for opt-in idempotency.
	DropCollection(ctx context.Context, name string, opts ...Option) error

	// AlterCollection applies ops to the existing collection. The
	// driver decides how to apply the batch (one combined ALTER
	// statement on PostgreSQL; a sequence on SQLite; etc.).
	// Transactional drivers (advertised via TransactionalDDL) MUST
	// roll back on partial failure; non-transactional drivers MAY
	// return *PartialSuccessError listing applied/failed/not-attempted ops.
	AlterCollection(ctx context.Context, name string, ops ...AlterOp) error
}
