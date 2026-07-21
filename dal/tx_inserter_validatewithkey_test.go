package dal

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/record"
)

type dataWithValidate struct{ ok bool }

func (d dataWithValidate) ValidateWithKey(_ *record.Key) error {
	if d.ok {
		return nil
	}
	return errors.New("validate failed")
}

func TestInsertWithIdGenerator_ValidateWithKey(t *testing.T) {
	ctx := context.Background()
	gen := func(ctx context.Context, r record.Record) error {
		r.Key().ID = "id1"
		return nil
	}
	exists := func(k *record.Key) error { return record.ErrRecordNotFound }
	insert := func(r record.Record) error { return nil }

	// success path
	r1 := record.NewRecordWithData(record.NewKeyWithID("Kind", ""), dataWithValidate{ok: true})
	if err := InsertWithIdGenerator(ctx, r1, gen, 1, exists, insert); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// validation error path
	r2 := record.NewRecordWithData(record.NewKeyWithID("Kind", ""), dataWithValidate{ok: false})
	err := InsertWithIdGenerator(ctx, r2, gen, 1, exists, insert)
	if err == nil || err.Error() == "" {
		t.Fatalf("expected validation error, got: %v", err)
	}
}
