mockgen github.com/dal-go/dalgo/update Update > mock_update/update.go
mockgen github.com/dal-go/dalgo/dal Schema > mock_dal/schema.go
mockgen github.com/dal-go/dalgo/dal DB > mock_dal/db.go
mockgen github.com/dal-go/dalgo/dal Reader > mock_dal/reader.go
mockgen github.com/dal-go/dalgo/dal ReadSession,WriteSession,ReadwriteSession > mock_dal/sessions.go
mockgen github.com/dal-go/dalgo/dal Transaction,ReadTransaction,ReadwriteTransaction > mock_dal/transactions.go
mockgen github.com/dal-go/dalgo/dal TransactionCoordinator,ReadTransactionCoordinator,ReadwriteTransactionCoordinator > mock_dal/transaction_coordinators.go
