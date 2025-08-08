package dal

import (
	"context"
	"errors"
	"testing"
)

type dataWithValidate struct{ ok bool }

func (d dataWithValidate) ValidateWithKey(key *Key) error {
	if d.ok {
		return nil
	}
	return errors.New("validate failed")
}

func TestInsertWithIdGenerator_ValidateWithKey(t *testing.T) {
	ctx := context.Background()
	gen := func(ctx context.Context, r Record) error {
		r.Key().ID = "id1"
		return nil
	}
	exists := func(k *Key) error { return ErrRecordNotFound }
	insert := func(r Record) error { return nil }

	// success path
 r1 := NewRecordWithData(NewKeyWithID("Kind", ""), dataWithValidate{ok: true})
	if err := InsertWithIdGenerator(ctx, r1, gen, 1, exists, insert); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// validation error path
 r2 := NewRecordWithData(NewKeyWithID("Kind", ""), dataWithValidate{ok: false})
	err := InsertWithIdGenerator(ctx, r2, gen, 1, exists, insert)
	if err == nil || err.Error() == "" {
		t.Fatalf("expected validation error, got: %v", err)
	}
}
