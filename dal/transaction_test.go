package dal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

//func TestWithPassword(t *testing.T) {
//	const password = "test-pwd"
//	txOptions := NewTransactionOptions(WithPassword(password))
//	if txOptions.Password() != password {
//		t.Errorf("unexpected password")
//	}
//}

func TestWithReadonly(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		txOptions := NewTransactionOptions(TxWithReadonly())
		if !txOptions.IsReadonly() {
			t.Errorf("expected to be readonly")
		}
	})
	t.Run("false", func(t *testing.T) {
		txOptions := NewTransactionOptions()
		if txOptions.IsReadonly() {
			t.Errorf("expected to be readonly")
		}
	})
}

func TestWithAttempts(t *testing.T) {
	t.Run("not_set", func(t *testing.T) {
		txOptions := NewTransactionOptions()
		assert.Equal(t, 0, txOptions.Attempts())
	})
	t.Run("set_to_3", func(t *testing.T) {
		txOptions := NewTransactionOptions(TxWithAttempts(3))
		assert.Equal(t, 3, txOptions.Attempts())
	})
}

func TestWithCrossGroup(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		txOptions := NewTransactionOptions(TxWithCrossGroup())
		if !txOptions.IsCrossGroup() {
			t.Errorf("expected to be true")
		}
	})
	t.Run("false", func(t *testing.T) {
		txOptions := NewTransactionOptions()
		if txOptions.IsCrossGroup() {
			t.Errorf("expected to be false")
		}
	})
}

func TestNewTransactionOptions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		options := NewTransactionOptions()
		if options.IsReadonly() {
			t.Errorf("expected to be not readonly")
		}
		//if password := options.Password(); password != "" {
		//	t.Error("expected not to have a password, got: "+password)
		//}
	})
	t.Run("readonly_cross_group_5_attempts", func(t *testing.T) {
		options := NewTransactionOptions(
			TxWithReadonly(),
			TxWithCrossGroup(),
			TxWithAttempts(5),
		)
		assert.True(t, options.IsReadonly())
		assert.True(t, options.IsCrossGroup())
		assert.Equal(t, 5, options.Attempts())
	})
	//t.Run("password", func(t *testing.T) {
	//	const expectedPassword = "test-pwd"
	//	options := NewTransactionOptions(WithPassword(expectedPassword))
	//	if options.IsReadonly() {
	//		t.Errorf("expected not to be readonly")
	//	}
	//	if password := options.Password(); password != expectedPassword {
	//		t.Errorf("expected not to have a password equal to %v, got: %v", expectedPassword, password)
	//	}
	//})
}

type mockTx struct {
	name    string
	options TransactionOptions
}

func (t mockTx) Options() TransactionOptions { return t.options }
func (t mockTx) Name() string                { return t.name }

func TestGetTransaction(t *testing.T) {
	tx := mockTx{options: NewTransactionOptions()}
	for _, tt := range []struct {
		name     string
		ctx      context.Context
		expected Transaction
	}{
		{
			name:     "background",
			ctx:      context.Background(),
			expected: nil,
		},
		{
			name:     "nil",
			ctx:      NewContextWithTransaction(context.Background(), nil),
			expected: nil,
		},
		{
			name:     "with_transaction",
			ctx:      NewContextWithTransaction(context.Background(), tx),
			expected: tx,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actual := GetTransaction(tt.ctx)
			if actual != tt.expected {
				t.Errorf("expected %v, got: %v", tt.expected, actual)
			}
		})
	}
}

func TestGetNonTransactionalContext(t *testing.T) {
	expected := mockTx{options: NewTransactionOptions()}
	ctx := context.Background()
	txCtx := NewContextWithTransaction(ctx, expected)
	actual := GetNonTransactionalContext(txCtx)
	if actual != ctx {
		t.Errorf("transactional context does not provide original context")
	}
}

func TestTxWithIsolationLevel(t *testing.T) {
	for _, tt := range []struct {
		name              string
		txIsolationLevel1 TxIsolationLevel
		txIsolationLevel2 TxIsolationLevel
		shouldPanic       bool
	}{
		{
			name:              "TxUnspecified",
			txIsolationLevel1: TxUnspecified,
			shouldPanic:       true,
		},
		{
			name:              "TxReadCommitted",
			txIsolationLevel1: TxReadCommitted,
			shouldPanic:       false,
		},
		{
			name:              "TxReadUncommitted",
			txIsolationLevel1: TxReadUncommitted,
			shouldPanic:       false,
		},
		{
			name:              "TxRepeatableRead",
			txIsolationLevel1: TxRepeatableRead,
			shouldPanic:       false,
		},
		{
			name:              "TxSerializable",
			txIsolationLevel1: TxSerializable,
			shouldPanic:       false,
		},
		{
			name:              "TxSnapshot",
			txIsolationLevel1: TxSnapshot,
			shouldPanic:       false,
		},
		{
			name:              "TxChaos",
			txIsolationLevel1: TxChaos,
			shouldPanic:       false,
		},
		{
			name:              "twice_same",
			txIsolationLevel1: TxReadCommitted,
			txIsolationLevel2: TxReadCommitted,
			shouldPanic:       true,
		},
		{
			name:              "twice_different",
			txIsolationLevel1: TxReadUncommitted,
			txIsolationLevel2: TxReadCommitted,
			shouldPanic:       true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldPanic {
				defer func() {
					if r := recover(); r != nil && !tt.shouldPanic {
						t.Errorf("unexpected panic: %v", r)
					}
				}()
			}
			to := new(txOptions)
			o := TxWithIsolationLevel(tt.txIsolationLevel1)
			o(to)
			if tt.txIsolationLevel2 != TxUnspecified {
				o = TxWithIsolationLevel(tt.txIsolationLevel2)
				o(to)
			}
			assert.Equal(t, tt.txIsolationLevel1, to.isolationLevel)
		})
	}
}

func TestTxOptions(t *testing.T) {
	for _, tt := range []struct {
		name        string
		txOptions   *txOptions
		shouldPanic bool
	}{
		{name: "nil", shouldPanic: true, txOptions: nil},
		{name: "attempts", txOptions: &txOptions{attempts: 1}},
		{name: "isCrossGroup", txOptions: &txOptions{isCrossGroup: true}},
		{name: "isReadonly", txOptions: &txOptions{isReadonly: true}},
		{name: "isolationLevel", txOptions: &txOptions{isolationLevel: TxReadCommitted}},
		{name: "password", txOptions: &txOptions{password: "test-pwd"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("expected panic")
					}
				}()
			}
			assert.Equal(t, tt.txOptions.attempts, tt.txOptions.Attempts())
			assert.Equal(t, tt.txOptions.isCrossGroup, tt.txOptions.IsCrossGroup())
			assert.Equal(t, tt.txOptions.isReadonly, tt.txOptions.IsReadonly())
			assert.Equal(t, tt.txOptions.isolationLevel, tt.txOptions.IsolationLevel())
			assert.Equal(t, tt.txOptions.password, tt.txOptions.Password())
		})
	}
}

func TestTxWithName(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		to := new(txOptions)
		o := TxWithName("")
		o(to)
		assert.Equal(t, "", to.Name())
	})
	t.Run("non_empty", func(t *testing.T) {
		to := new(txOptions)
		o := TxWithName("my-tx")
		o(to)
		assert.Equal(t, "my-tx", to.Name())
	})
	t.Run("twice_overwrite", func(t *testing.T) {
		to := new(txOptions)
		TxWithName("first")(to)
		TxWithName("second")(to)
		assert.Equal(t, "second", to.Name())
	})
	t.Run("with_NewTransactionOptions", func(t *testing.T) {
		opts := NewTransactionOptions(TxWithName("builder-tx"))
		name := opts.(txOptions).Name()
		assert.Equal(t, "builder-tx", name)
	})
}
