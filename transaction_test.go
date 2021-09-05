package db

import "testing"

func TestWithPassword(t *testing.T) {
	const password = "test-pwd"
	txOptions := NewTransactionOptions(WithPassword(password))
	if txOptions.Password() != password {
		t.Errorf("unexpected password")
	}
}

func TestWithReadonly(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		txOptions := NewTransactionOptions(WithReadonly())
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

func TestNewTransactionOptions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		options := NewTransactionOptions()
		if options.IsReadonly() {
			t.Errorf("expected to be not readonly")
		}
		if password := options.Password(); password != "" {
			t.Errorf("expected not to have a password, got: %v", password)
		}
	})
	t.Run("readonly", func(t *testing.T) {
		options := NewTransactionOptions(WithReadonly())
		if !options.IsReadonly() {
			t.Errorf("expected to be readonly")
		}
		if password := options.Password(); password != "" {
			t.Errorf("expected not to have a password, got: %v", password)
		}
	})
	t.Run("password", func(t *testing.T) {
		const expectedPassword = "test-pwd"
		options := NewTransactionOptions(WithPassword(expectedPassword))
		if options.IsReadonly() {
			t.Errorf("expected not to be readonly")
		}
		if password := options.Password(); password != expectedPassword {
			t.Errorf("expected not to have a password equal to %v, got: %v", expectedPassword, password)
		}
	})
}
