package dal

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func TestNewErrNotFoundByKey(t *testing.T) {
	type args struct {
		key   *Key
		cause error
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "ErrRecordNotFound",
			args: args{
				key:   NewKeyWithID("Foo", "bar"),
				cause: ErrRecordNotFound,
			},
		},
		{
			name: "nil",
			args: args{
				key:   NewKeyWithID("Foo", "bar"),
				cause: nil,
			},
		},
		{
			name: "some_error",
			args: args{
				key:   NewKeyWithID("Foo", "bar"),
				cause: errors.New("some error"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewErrNotFoundByKey(tt.args.key, tt.args.cause)
			assert.True(t, IsNotFound(err))

			err2 := fmt.Errorf("wrapper: %w", err)
			assert.True(t, IsNotFound(err2))
		})
	}
}

func TestErrNotImplementedYet(t *testing.T) {
	assert.Equal(t, "not implemented yet", ErrNotImplementedYet.Error())
	err := fmt.Errorf("%w: justification and ETA", ErrNotImplementedYet)
	assert.True(t, errors.Is(err, ErrNotImplementedYet))
}

func TestErrNotSupported(t *testing.T) {
	assert.Equal(t, "not supported", ErrNotSupported.Error())
	err := fmt.Errorf("%w: explanation why", ErrNotSupported)
	assert.True(t, errors.Is(err, ErrNotSupported))
}

func TestNewRollbackError(t *testing.T) {
	err := NewRollbackError(errors.New("some rollback error"), errors.New("some original error"))
	rollbackErr, isRollbackError := err.(rollbackError)
	assert.True(t, true, isRollbackError)
	assert.NotNil(t, rollbackErr)
	assert.True(t, strings.Contains(err.Error(), "some rollback error"))
	assert.True(t, strings.Contains(err.Error(), "some original error"))
}

func TestErrDuplicateUser_Error(t *testing.T) {
	err := ErrDuplicateUser{SearchCriteria: "criteria1", DuplicateUserIDs: []string{"id1", "id2"}}
	s := err.Error()
	if s == "" {
		t.Fatal("Expected non-empty string")
	}
	if !strings.Contains(s, err.SearchCriteria) {
		t.Errorf("Expected %v to contain %v", s, err.SearchCriteria)
	}
	for _, uid := range err.DuplicateUserIDs {
		if !strings.Contains(s, uid) {
			t.Errorf("Expected %v to contain %v", s, uid)
		}
	}
}

func TestIsNotFound(t *testing.T) {
	type test struct {
		name string
		err  error
		want bool
	}
	tests := []test{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "nil",
			err:  errors.New("some error"),
			want: false,
		},
		{
			name: "ErrRecordNotFound",
			err:  ErrRecordNotFound,
			want: true,
		},
		{
			name: "errNotFoundByKey",
			err:  errNotFoundByKey{cause: ErrRecordNotFound},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsNotFound(tt.err))
		})
	}
}

func TestErrNotFoundByKey_Key(t *testing.T) {
	err := errNotFoundByKey{
		key: &Key{ID: "id1", collection: "records"},
	}
	key := err.Key()
	assert.NotNil(t, key)
	assert.Equal(t, "id1", key.ID)
	assert.Equal(t, "records", key.collection)
}

func TestErrNotFoundByKey_Cause(t *testing.T) {
	cause := errors.New("some cause")
	err := errNotFoundByKey{
		cause: cause,
	}
	assert.Equal(t, cause, err.Cause())
}

func TestErrNotFoundByKey_Error(t *testing.T) {
	type test struct {
		name string
		err  errNotFoundByKey
	}
	tests := []test{
		{
			name: "no_cause",
			err:  errNotFoundByKey{key: &Key{ID: "id1", collection: "records"}},
		},
		{
			name: "cause_is_ErrRecordNotFound",
			err: errNotFoundByKey{
				key:   &Key{ID: "id1", collection: "records"},
				cause: ErrRecordNotFound,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.err.Error()
			assert.True(t, strings.Contains(s, tt.err.key.ID.(string)))
			assert.True(t, strings.Contains(s, tt.err.key.collection))
			if tt.err.cause == nil {
				assert.True(t, strings.Contains(s, "record not found"))
			} else {
				assert.True(t, strings.Contains(s, tt.err.cause.Error()))
			}
		})
	}
}

func TestRollbackError_Error(t *testing.T) {
	type test struct {
		name string
		err  rollbackError
	}
	tests := []test{
		{
			name: "empty",
			err:  rollbackError{},
		},
		{
			name: "with_original_error",
			err:  rollbackError{originalErr: errors.New("some error")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.err.Error()
			assert.True(t, strings.Contains(s, "rollback failed"), "expected 'rollback error' in %v", s)
			if tt.err.originalErr != nil {
				assert.True(t, strings.Contains(s, "original error"))
				assert.True(t, strings.Contains(s, tt.err.originalErr.Error()))
			}
		})
	}
}
