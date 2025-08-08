package mock_dal

import "github.com/dal-go/dalgo/dal"

var _ dal.DB = (*MockDB)(nil)
var _ dal.ReadSession = (*MockReadSession)(nil)
var _ dal.WriteSession = (*MockWriteSession)(nil)
var _ dal.ReadwriteSession = (*MockReadwriteSession)(nil)
var _ dal.ReadTransaction = (*MockReadTransaction)(nil)
var _ dal.ReadwriteTransaction = (*MockReadwriteTransaction)(nil)
var _ dal.TransactionCoordinator = (*MockTransactionCoordinator)(nil)
var _ dal.ReadTransactionCoordinator = (*MockReadTransactionCoordinator)(nil)
var _ dal.ReadwriteTransactionCoordinator = (*MockReadwriteTransactionCoordinator)(nil)
