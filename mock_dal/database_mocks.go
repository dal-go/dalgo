// Code generated by MockGen. DO NOT EDIT.
// Source: ../dal/database.go

// Package mock_dal is a generated GoMock package.
package mock_dal

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	dal "github.com/strongo/dalgo/dal"
)

// MockDatabase is a mock of Database interface.
type MockDatabase struct {
	ctrl     *gomock.Controller
	recorder *MockDatabaseMockRecorder
}

// MockDatabaseMockRecorder is the mock recorder for MockDatabase.
type MockDatabaseMockRecorder struct {
	mock *MockDatabase
}

// NewMockDatabase creates a new mock instance.
func NewMockDatabase(ctrl *gomock.Controller) *MockDatabase {
	mock := &MockDatabase{ctrl: ctrl}
	mock.recorder = &MockDatabaseMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDatabase) EXPECT() *MockDatabaseMockRecorder {
	return m.recorder
}

// Get mocks base method.
func (m *MockDatabase) Get(ctx context.Context, record dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", ctx, record)
	ret0, _ := ret[0].(error)
	return ret0
}

// Get indicates an expected call of Get.
func (mr *MockDatabaseMockRecorder) Get(ctx, record interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockDatabase)(nil).Get), ctx, record)
}

// GetMulti mocks base method.
func (m *MockDatabase) GetMulti(ctx context.Context, records []dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMulti", ctx, records)
	ret0, _ := ret[0].(error)
	return ret0
}

// GetMulti indicates an expected call of GetMulti.
func (mr *MockDatabaseMockRecorder) GetMulti(ctx, records interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMulti", reflect.TypeOf((*MockDatabase)(nil).GetMulti), ctx, records)
}

// RunReadonlyTransaction mocks base method.
func (m *MockDatabase) RunReadonlyTransaction(ctx context.Context, f dal.ROTxWorker, options ...dal.TransactionOption) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, f}
	for _, a := range options {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "RunReadonlyTransaction", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// RunReadonlyTransaction indicates an expected call of RunReadonlyTransaction.
func (mr *MockDatabaseMockRecorder) RunReadonlyTransaction(ctx, f interface{}, options ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, f}, options...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RunReadonlyTransaction", reflect.TypeOf((*MockDatabase)(nil).RunReadonlyTransaction), varargs...)
}

// RunReadwriteTransaction mocks base method.
func (m *MockDatabase) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, options ...dal.TransactionOption) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, f}
	for _, a := range options {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "RunReadwriteTransaction", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// RunReadwriteTransaction indicates an expected call of RunReadwriteTransaction.
func (mr *MockDatabaseMockRecorder) RunReadwriteTransaction(ctx, f interface{}, options ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, f}, options...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RunReadwriteTransaction", reflect.TypeOf((*MockDatabase)(nil).RunReadwriteTransaction), varargs...)
}

// Select mocks base method.
func (m *MockDatabase) Select(ctx context.Context, query dal.Select) (dal.Reader, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Select", ctx, query)
	ret0, _ := ret[0].(dal.Reader)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Select indicates an expected call of Select.
func (mr *MockDatabaseMockRecorder) Select(ctx, query interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Select", reflect.TypeOf((*MockDatabase)(nil).Select), ctx, query)
}

// MockTransactionCoordinator is a mock of TransactionCoordinator interface.
type MockTransactionCoordinator struct {
	ctrl     *gomock.Controller
	recorder *MockTransactionCoordinatorMockRecorder
}

// MockTransactionCoordinatorMockRecorder is the mock recorder for MockTransactionCoordinator.
type MockTransactionCoordinatorMockRecorder struct {
	mock *MockTransactionCoordinator
}

// NewMockTransactionCoordinator creates a new mock instance.
func NewMockTransactionCoordinator(ctrl *gomock.Controller) *MockTransactionCoordinator {
	mock := &MockTransactionCoordinator{ctrl: ctrl}
	mock.recorder = &MockTransactionCoordinatorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockTransactionCoordinator) EXPECT() *MockTransactionCoordinatorMockRecorder {
	return m.recorder
}

// RunReadonlyTransaction mocks base method.
func (m *MockTransactionCoordinator) RunReadonlyTransaction(ctx context.Context, f dal.ROTxWorker, options ...dal.TransactionOption) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, f}
	for _, a := range options {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "RunReadonlyTransaction", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// RunReadonlyTransaction indicates an expected call of RunReadonlyTransaction.
func (mr *MockTransactionCoordinatorMockRecorder) RunReadonlyTransaction(ctx, f interface{}, options ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, f}, options...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RunReadonlyTransaction", reflect.TypeOf((*MockTransactionCoordinator)(nil).RunReadonlyTransaction), varargs...)
}

// RunReadwriteTransaction mocks base method.
func (m *MockTransactionCoordinator) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, options ...dal.TransactionOption) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, f}
	for _, a := range options {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "RunReadwriteTransaction", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// RunReadwriteTransaction indicates an expected call of RunReadwriteTransaction.
func (mr *MockTransactionCoordinatorMockRecorder) RunReadwriteTransaction(ctx, f interface{}, options ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, f}, options...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RunReadwriteTransaction", reflect.TypeOf((*MockTransactionCoordinator)(nil).RunReadwriteTransaction), varargs...)
}

// MockReadTransactionCoordinator is a mock of ReadTransactionCoordinator interface.
type MockReadTransactionCoordinator struct {
	ctrl     *gomock.Controller
	recorder *MockReadTransactionCoordinatorMockRecorder
}

// MockReadTransactionCoordinatorMockRecorder is the mock recorder for MockReadTransactionCoordinator.
type MockReadTransactionCoordinatorMockRecorder struct {
	mock *MockReadTransactionCoordinator
}

// NewMockReadTransactionCoordinator creates a new mock instance.
func NewMockReadTransactionCoordinator(ctrl *gomock.Controller) *MockReadTransactionCoordinator {
	mock := &MockReadTransactionCoordinator{ctrl: ctrl}
	mock.recorder = &MockReadTransactionCoordinatorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockReadTransactionCoordinator) EXPECT() *MockReadTransactionCoordinatorMockRecorder {
	return m.recorder
}

// RunReadonlyTransaction mocks base method.
func (m *MockReadTransactionCoordinator) RunReadonlyTransaction(ctx context.Context, f dal.ROTxWorker, options ...dal.TransactionOption) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, f}
	for _, a := range options {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "RunReadonlyTransaction", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// RunReadonlyTransaction indicates an expected call of RunReadonlyTransaction.
func (mr *MockReadTransactionCoordinatorMockRecorder) RunReadonlyTransaction(ctx, f interface{}, options ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, f}, options...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RunReadonlyTransaction", reflect.TypeOf((*MockReadTransactionCoordinator)(nil).RunReadonlyTransaction), varargs...)
}

// MockReadwriteTransactionCoordinator is a mock of ReadwriteTransactionCoordinator interface.
type MockReadwriteTransactionCoordinator struct {
	ctrl     *gomock.Controller
	recorder *MockReadwriteTransactionCoordinatorMockRecorder
}

// MockReadwriteTransactionCoordinatorMockRecorder is the mock recorder for MockReadwriteTransactionCoordinator.
type MockReadwriteTransactionCoordinatorMockRecorder struct {
	mock *MockReadwriteTransactionCoordinator
}

// NewMockReadwriteTransactionCoordinator creates a new mock instance.
func NewMockReadwriteTransactionCoordinator(ctrl *gomock.Controller) *MockReadwriteTransactionCoordinator {
	mock := &MockReadwriteTransactionCoordinator{ctrl: ctrl}
	mock.recorder = &MockReadwriteTransactionCoordinatorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockReadwriteTransactionCoordinator) EXPECT() *MockReadwriteTransactionCoordinatorMockRecorder {
	return m.recorder
}

// RunReadwriteTransaction mocks base method.
func (m *MockReadwriteTransactionCoordinator) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, options ...dal.TransactionOption) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, f}
	for _, a := range options {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "RunReadwriteTransaction", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// RunReadwriteTransaction indicates an expected call of RunReadwriteTransaction.
func (mr *MockReadwriteTransactionCoordinatorMockRecorder) RunReadwriteTransaction(ctx, f interface{}, options ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, f}, options...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RunReadwriteTransaction", reflect.TypeOf((*MockReadwriteTransactionCoordinator)(nil).RunReadwriteTransaction), varargs...)
}

// MockTransaction is a mock of Transaction interface.
type MockTransaction struct {
	ctrl     *gomock.Controller
	recorder *MockTransactionMockRecorder
}

// MockTransactionMockRecorder is the mock recorder for MockTransaction.
type MockTransactionMockRecorder struct {
	mock *MockTransaction
}

// NewMockTransaction creates a new mock instance.
func NewMockTransaction(ctrl *gomock.Controller) *MockTransaction {
	mock := &MockTransaction{ctrl: ctrl}
	mock.recorder = &MockTransactionMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockTransaction) EXPECT() *MockTransactionMockRecorder {
	return m.recorder
}

// Options mocks base method.
func (m *MockTransaction) Options() dal.TransactionOptions {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Options")
	ret0, _ := ret[0].(dal.TransactionOptions)
	return ret0
}

// Options indicates an expected call of Options.
func (mr *MockTransactionMockRecorder) Options() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Options", reflect.TypeOf((*MockTransaction)(nil).Options))
}

// MockReadTransaction is a mock of ReadTransaction interface.
type MockReadTransaction struct {
	ctrl     *gomock.Controller
	recorder *MockReadTransactionMockRecorder
}

// MockReadTransactionMockRecorder is the mock recorder for MockReadTransaction.
type MockReadTransactionMockRecorder struct {
	mock *MockReadTransaction
}

// NewMockReadTransaction creates a new mock instance.
func NewMockReadTransaction(ctrl *gomock.Controller) *MockReadTransaction {
	mock := &MockReadTransaction{ctrl: ctrl}
	mock.recorder = &MockReadTransactionMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockReadTransaction) EXPECT() *MockReadTransactionMockRecorder {
	return m.recorder
}

// Get mocks base method.
func (m *MockReadTransaction) Get(ctx context.Context, record dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", ctx, record)
	ret0, _ := ret[0].(error)
	return ret0
}

// Get indicates an expected call of Get.
func (mr *MockReadTransactionMockRecorder) Get(ctx, record interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockReadTransaction)(nil).Get), ctx, record)
}

// GetMulti mocks base method.
func (m *MockReadTransaction) GetMulti(ctx context.Context, records []dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMulti", ctx, records)
	ret0, _ := ret[0].(error)
	return ret0
}

// GetMulti indicates an expected call of GetMulti.
func (mr *MockReadTransactionMockRecorder) GetMulti(ctx, records interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMulti", reflect.TypeOf((*MockReadTransaction)(nil).GetMulti), ctx, records)
}

// Options mocks base method.
func (m *MockReadTransaction) Options() dal.TransactionOptions {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Options")
	ret0, _ := ret[0].(dal.TransactionOptions)
	return ret0
}

// Options indicates an expected call of Options.
func (mr *MockReadTransactionMockRecorder) Options() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Options", reflect.TypeOf((*MockReadTransaction)(nil).Options))
}

// Select mocks base method.
func (m *MockReadTransaction) Select(ctx context.Context, query dal.Select) (dal.Reader, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Select", ctx, query)
	ret0, _ := ret[0].(dal.Reader)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Select indicates an expected call of Select.
func (mr *MockReadTransactionMockRecorder) Select(ctx, query interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Select", reflect.TypeOf((*MockReadTransaction)(nil).Select), ctx, query)
}

// MockReadwriteTransaction is a mock of ReadwriteTransaction interface.
type MockReadwriteTransaction struct {
	ctrl     *gomock.Controller
	recorder *MockReadwriteTransactionMockRecorder
}

// MockReadwriteTransactionMockRecorder is the mock recorder for MockReadwriteTransaction.
type MockReadwriteTransactionMockRecorder struct {
	mock *MockReadwriteTransaction
}

// NewMockReadwriteTransaction creates a new mock instance.
func NewMockReadwriteTransaction(ctrl *gomock.Controller) *MockReadwriteTransaction {
	mock := &MockReadwriteTransaction{ctrl: ctrl}
	mock.recorder = &MockReadwriteTransactionMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockReadwriteTransaction) EXPECT() *MockReadwriteTransactionMockRecorder {
	return m.recorder
}

// Delete mocks base method.
func (m *MockReadwriteTransaction) Delete(ctx context.Context, key *dal.Key) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Delete", ctx, key)
	ret0, _ := ret[0].(error)
	return ret0
}

// Delete indicates an expected call of Delete.
func (mr *MockReadwriteTransactionMockRecorder) Delete(ctx, key interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockReadwriteTransaction)(nil).Delete), ctx, key)
}

// DeleteMulti mocks base method.
func (m *MockReadwriteTransaction) DeleteMulti(ctx context.Context, keys []*dal.Key) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteMulti", ctx, keys)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteMulti indicates an expected call of DeleteMulti.
func (mr *MockReadwriteTransactionMockRecorder) DeleteMulti(ctx, keys interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteMulti", reflect.TypeOf((*MockReadwriteTransaction)(nil).DeleteMulti), ctx, keys)
}

// Get mocks base method.
func (m *MockReadwriteTransaction) Get(ctx context.Context, record dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", ctx, record)
	ret0, _ := ret[0].(error)
	return ret0
}

// Get indicates an expected call of Get.
func (mr *MockReadwriteTransactionMockRecorder) Get(ctx, record interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockReadwriteTransaction)(nil).Get), ctx, record)
}

// GetMulti mocks base method.
func (m *MockReadwriteTransaction) GetMulti(ctx context.Context, records []dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMulti", ctx, records)
	ret0, _ := ret[0].(error)
	return ret0
}

// GetMulti indicates an expected call of GetMulti.
func (mr *MockReadwriteTransactionMockRecorder) GetMulti(ctx, records interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMulti", reflect.TypeOf((*MockReadwriteTransaction)(nil).GetMulti), ctx, records)
}

// Insert mocks base method.
func (m *MockReadwriteTransaction) Insert(c context.Context, record dal.Record, opts ...dal.InsertOption) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{c, record}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Insert", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// Insert indicates an expected call of Insert.
func (mr *MockReadwriteTransactionMockRecorder) Insert(c, record interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{c, record}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Insert", reflect.TypeOf((*MockReadwriteTransaction)(nil).Insert), varargs...)
}

// Options mocks base method.
func (m *MockReadwriteTransaction) Options() dal.TransactionOptions {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Options")
	ret0, _ := ret[0].(dal.TransactionOptions)
	return ret0
}

// Options indicates an expected call of Options.
func (mr *MockReadwriteTransactionMockRecorder) Options() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Options", reflect.TypeOf((*MockReadwriteTransaction)(nil).Options))
}

// Select mocks base method.
func (m *MockReadwriteTransaction) Select(ctx context.Context, query dal.Select) (dal.Reader, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Select", ctx, query)
	ret0, _ := ret[0].(dal.Reader)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Select indicates an expected call of Select.
func (mr *MockReadwriteTransactionMockRecorder) Select(ctx, query interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Select", reflect.TypeOf((*MockReadwriteTransaction)(nil).Select), ctx, query)
}

// Set mocks base method.
func (m *MockReadwriteTransaction) Set(ctx context.Context, record dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Set", ctx, record)
	ret0, _ := ret[0].(error)
	return ret0
}

// Set indicates an expected call of Set.
func (mr *MockReadwriteTransactionMockRecorder) Set(ctx, record interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Set", reflect.TypeOf((*MockReadwriteTransaction)(nil).Set), ctx, record)
}

// SetMulti mocks base method.
func (m *MockReadwriteTransaction) SetMulti(ctx context.Context, records []dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetMulti", ctx, records)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetMulti indicates an expected call of SetMulti.
func (mr *MockReadwriteTransactionMockRecorder) SetMulti(ctx, records interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetMulti", reflect.TypeOf((*MockReadwriteTransaction)(nil).SetMulti), ctx, records)
}

// Update mocks base method.
func (m *MockReadwriteTransaction) Update(ctx context.Context, key *dal.Key, updates []dal.Update, preconditions ...dal.Precondition) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, key, updates}
	for _, a := range preconditions {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Update", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// Update indicates an expected call of Update.
func (mr *MockReadwriteTransactionMockRecorder) Update(ctx, key, updates interface{}, preconditions ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, key, updates}, preconditions...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Update", reflect.TypeOf((*MockReadwriteTransaction)(nil).Update), varargs...)
}

// UpdateMulti mocks base method.
func (m *MockReadwriteTransaction) UpdateMulti(c context.Context, keys []*dal.Key, updates []dal.Update, preconditions ...dal.Precondition) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{c, keys, updates}
	for _, a := range preconditions {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "UpdateMulti", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateMulti indicates an expected call of UpdateMulti.
func (mr *MockReadwriteTransactionMockRecorder) UpdateMulti(c, keys, updates interface{}, preconditions ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{c, keys, updates}, preconditions...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateMulti", reflect.TypeOf((*MockReadwriteTransaction)(nil).UpdateMulti), varargs...)
}

// MockReadSession is a mock of ReadSession interface.
type MockReadSession struct {
	ctrl     *gomock.Controller
	recorder *MockReadSessionMockRecorder
}

// MockReadSessionMockRecorder is the mock recorder for MockReadSession.
type MockReadSessionMockRecorder struct {
	mock *MockReadSession
}

// NewMockReadSession creates a new mock instance.
func NewMockReadSession(ctrl *gomock.Controller) *MockReadSession {
	mock := &MockReadSession{ctrl: ctrl}
	mock.recorder = &MockReadSessionMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockReadSession) EXPECT() *MockReadSessionMockRecorder {
	return m.recorder
}

// Get mocks base method.
func (m *MockReadSession) Get(ctx context.Context, record dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", ctx, record)
	ret0, _ := ret[0].(error)
	return ret0
}

// Get indicates an expected call of Get.
func (mr *MockReadSessionMockRecorder) Get(ctx, record interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockReadSession)(nil).Get), ctx, record)
}

// GetMulti mocks base method.
func (m *MockReadSession) GetMulti(ctx context.Context, records []dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMulti", ctx, records)
	ret0, _ := ret[0].(error)
	return ret0
}

// GetMulti indicates an expected call of GetMulti.
func (mr *MockReadSessionMockRecorder) GetMulti(ctx, records interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMulti", reflect.TypeOf((*MockReadSession)(nil).GetMulti), ctx, records)
}

// Select mocks base method.
func (m *MockReadSession) Select(ctx context.Context, query dal.Select) (dal.Reader, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Select", ctx, query)
	ret0, _ := ret[0].(dal.Reader)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Select indicates an expected call of Select.
func (mr *MockReadSessionMockRecorder) Select(ctx, query interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Select", reflect.TypeOf((*MockReadSession)(nil).Select), ctx, query)
}

// MockReadwriteSession is a mock of ReadwriteSession interface.
type MockReadwriteSession struct {
	ctrl     *gomock.Controller
	recorder *MockReadwriteSessionMockRecorder
}

// MockReadwriteSessionMockRecorder is the mock recorder for MockReadwriteSession.
type MockReadwriteSessionMockRecorder struct {
	mock *MockReadwriteSession
}

// NewMockReadwriteSession creates a new mock instance.
func NewMockReadwriteSession(ctrl *gomock.Controller) *MockReadwriteSession {
	mock := &MockReadwriteSession{ctrl: ctrl}
	mock.recorder = &MockReadwriteSessionMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockReadwriteSession) EXPECT() *MockReadwriteSessionMockRecorder {
	return m.recorder
}

// Delete mocks base method.
func (m *MockReadwriteSession) Delete(ctx context.Context, key *dal.Key) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Delete", ctx, key)
	ret0, _ := ret[0].(error)
	return ret0
}

// Delete indicates an expected call of Delete.
func (mr *MockReadwriteSessionMockRecorder) Delete(ctx, key interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockReadwriteSession)(nil).Delete), ctx, key)
}

// DeleteMulti mocks base method.
func (m *MockReadwriteSession) DeleteMulti(ctx context.Context, keys []*dal.Key) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteMulti", ctx, keys)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteMulti indicates an expected call of DeleteMulti.
func (mr *MockReadwriteSessionMockRecorder) DeleteMulti(ctx, keys interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteMulti", reflect.TypeOf((*MockReadwriteSession)(nil).DeleteMulti), ctx, keys)
}

// Get mocks base method.
func (m *MockReadwriteSession) Get(ctx context.Context, record dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", ctx, record)
	ret0, _ := ret[0].(error)
	return ret0
}

// Get indicates an expected call of Get.
func (mr *MockReadwriteSessionMockRecorder) Get(ctx, record interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockReadwriteSession)(nil).Get), ctx, record)
}

// GetMulti mocks base method.
func (m *MockReadwriteSession) GetMulti(ctx context.Context, records []dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMulti", ctx, records)
	ret0, _ := ret[0].(error)
	return ret0
}

// GetMulti indicates an expected call of GetMulti.
func (mr *MockReadwriteSessionMockRecorder) GetMulti(ctx, records interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMulti", reflect.TypeOf((*MockReadwriteSession)(nil).GetMulti), ctx, records)
}

// Insert mocks base method.
func (m *MockReadwriteSession) Insert(c context.Context, record dal.Record, opts ...dal.InsertOption) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{c, record}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Insert", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// Insert indicates an expected call of Insert.
func (mr *MockReadwriteSessionMockRecorder) Insert(c, record interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{c, record}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Insert", reflect.TypeOf((*MockReadwriteSession)(nil).Insert), varargs...)
}

// Select mocks base method.
func (m *MockReadwriteSession) Select(ctx context.Context, query dal.Select) (dal.Reader, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Select", ctx, query)
	ret0, _ := ret[0].(dal.Reader)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Select indicates an expected call of Select.
func (mr *MockReadwriteSessionMockRecorder) Select(ctx, query interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Select", reflect.TypeOf((*MockReadwriteSession)(nil).Select), ctx, query)
}

// Set mocks base method.
func (m *MockReadwriteSession) Set(ctx context.Context, record dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Set", ctx, record)
	ret0, _ := ret[0].(error)
	return ret0
}

// Set indicates an expected call of Set.
func (mr *MockReadwriteSessionMockRecorder) Set(ctx, record interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Set", reflect.TypeOf((*MockReadwriteSession)(nil).Set), ctx, record)
}

// SetMulti mocks base method.
func (m *MockReadwriteSession) SetMulti(ctx context.Context, records []dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetMulti", ctx, records)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetMulti indicates an expected call of SetMulti.
func (mr *MockReadwriteSessionMockRecorder) SetMulti(ctx, records interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetMulti", reflect.TypeOf((*MockReadwriteSession)(nil).SetMulti), ctx, records)
}

// Update mocks base method.
func (m *MockReadwriteSession) Update(ctx context.Context, key *dal.Key, updates []dal.Update, preconditions ...dal.Precondition) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, key, updates}
	for _, a := range preconditions {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Update", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// Update indicates an expected call of Update.
func (mr *MockReadwriteSessionMockRecorder) Update(ctx, key, updates interface{}, preconditions ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, key, updates}, preconditions...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Update", reflect.TypeOf((*MockReadwriteSession)(nil).Update), varargs...)
}

// UpdateMulti mocks base method.
func (m *MockReadwriteSession) UpdateMulti(c context.Context, keys []*dal.Key, updates []dal.Update, preconditions ...dal.Precondition) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{c, keys, updates}
	for _, a := range preconditions {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "UpdateMulti", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateMulti indicates an expected call of UpdateMulti.
func (mr *MockReadwriteSessionMockRecorder) UpdateMulti(c, keys, updates interface{}, preconditions ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{c, keys, updates}, preconditions...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateMulti", reflect.TypeOf((*MockReadwriteSession)(nil).UpdateMulti), varargs...)
}

// MockWriteSession is a mock of WriteSession interface.
type MockWriteSession struct {
	ctrl     *gomock.Controller
	recorder *MockWriteSessionMockRecorder
}

// MockWriteSessionMockRecorder is the mock recorder for MockWriteSession.
type MockWriteSessionMockRecorder struct {
	mock *MockWriteSession
}

// NewMockWriteSession creates a new mock instance.
func NewMockWriteSession(ctrl *gomock.Controller) *MockWriteSession {
	mock := &MockWriteSession{ctrl: ctrl}
	mock.recorder = &MockWriteSessionMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockWriteSession) EXPECT() *MockWriteSessionMockRecorder {
	return m.recorder
}

// Delete mocks base method.
func (m *MockWriteSession) Delete(ctx context.Context, key *dal.Key) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Delete", ctx, key)
	ret0, _ := ret[0].(error)
	return ret0
}

// Delete indicates an expected call of Delete.
func (mr *MockWriteSessionMockRecorder) Delete(ctx, key interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockWriteSession)(nil).Delete), ctx, key)
}

// DeleteMulti mocks base method.
func (m *MockWriteSession) DeleteMulti(ctx context.Context, keys []*dal.Key) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteMulti", ctx, keys)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteMulti indicates an expected call of DeleteMulti.
func (mr *MockWriteSessionMockRecorder) DeleteMulti(ctx, keys interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteMulti", reflect.TypeOf((*MockWriteSession)(nil).DeleteMulti), ctx, keys)
}

// Insert mocks base method.
func (m *MockWriteSession) Insert(c context.Context, record dal.Record, opts ...dal.InsertOption) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{c, record}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Insert", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// Insert indicates an expected call of Insert.
func (mr *MockWriteSessionMockRecorder) Insert(c, record interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{c, record}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Insert", reflect.TypeOf((*MockWriteSession)(nil).Insert), varargs...)
}

// Set mocks base method.
func (m *MockWriteSession) Set(ctx context.Context, record dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Set", ctx, record)
	ret0, _ := ret[0].(error)
	return ret0
}

// Set indicates an expected call of Set.
func (mr *MockWriteSessionMockRecorder) Set(ctx, record interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Set", reflect.TypeOf((*MockWriteSession)(nil).Set), ctx, record)
}

// SetMulti mocks base method.
func (m *MockWriteSession) SetMulti(ctx context.Context, records []dal.Record) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetMulti", ctx, records)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetMulti indicates an expected call of SetMulti.
func (mr *MockWriteSessionMockRecorder) SetMulti(ctx, records interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetMulti", reflect.TypeOf((*MockWriteSession)(nil).SetMulti), ctx, records)
}

// Update mocks base method.
func (m *MockWriteSession) Update(ctx context.Context, key *dal.Key, updates []dal.Update, preconditions ...dal.Precondition) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, key, updates}
	for _, a := range preconditions {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Update", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// Update indicates an expected call of Update.
func (mr *MockWriteSessionMockRecorder) Update(ctx, key, updates interface{}, preconditions ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, key, updates}, preconditions...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Update", reflect.TypeOf((*MockWriteSession)(nil).Update), varargs...)
}

// UpdateMulti mocks base method.
func (m *MockWriteSession) UpdateMulti(c context.Context, keys []*dal.Key, updates []dal.Update, preconditions ...dal.Precondition) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{c, keys, updates}
	for _, a := range preconditions {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "UpdateMulti", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateMulti indicates an expected call of UpdateMulti.
func (mr *MockWriteSessionMockRecorder) UpdateMulti(c, keys, updates interface{}, preconditions ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{c, keys, updates}, preconditions...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateMulti", reflect.TypeOf((*MockWriteSession)(nil).UpdateMulti), varargs...)
}

// MockReader is a mock of Reader interface.
type MockReader struct {
	ctrl     *gomock.Controller
	recorder *MockReaderMockRecorder
}

// MockReaderMockRecorder is the mock recorder for MockReader.
type MockReaderMockRecorder struct {
	mock *MockReader
}

// NewMockReader creates a new mock instance.
func NewMockReader(ctrl *gomock.Controller) *MockReader {
	mock := &MockReader{ctrl: ctrl}
	mock.recorder = &MockReaderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockReader) EXPECT() *MockReaderMockRecorder {
	return m.recorder
}

// Next mocks base method.
func (m *MockReader) Next() (dal.Record, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Next")
	ret0, _ := ret[0].(dal.Record)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Next indicates an expected call of Next.
func (mr *MockReaderMockRecorder) Next() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Next", reflect.TypeOf((*MockReader)(nil).Next))
}
