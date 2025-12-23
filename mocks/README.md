# mocks4dalgo

A tests helper package that provides mocks for:

* [dalgo/dal](https://github.com/dal-go/dalgo/tree/main/dal) - `mocks4dal`

You should have [`gomock`](https://go.uber.org/mock) installed to regenerate mocks.

## [mocks4dal](mock_dal)

To regenerate [`mocks.go`](mock_dal/mocks.go) file, run the following command:

    $ mockgen -package mocks4dal github.com/dal-go/dalgo/dal Database,TransactionCoordinator,ReadTransactionCoordinator,ReadwriteTransactionCoordinator,Transaction,ReadTransaction,ReadwriteTransaction,ReadSession,WriteSession,ReadwriteSession > mocks4dal/mocks.go

