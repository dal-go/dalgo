package dalgo

import (
	"context"
	"fmt"
	"testing"
)

type foo struct {
	title string
}

func (foo foo) Kind() string {
	return "foo"
}

func (foo foo) Validate() error {
	if foo.title == "" {
		return fmt.Errorf("missing required field: title")
	}
	return nil
}

type inserterMock struct {
}

func TestVoidData(t *testing.T) {
	v := VoidData()
	if v == nil {
		t.Error("expected to be not nil")
	}
	switch v.(type) {
	case *o: // OK
		break
	default:
		t.Errorf("unexpected type of void data: %T", v)
	}
}

func (v inserterMock) Insert(c context.Context, record Record, options InsertOptions) error {
	if idGenerator := options.IDGenerator(); idGenerator != nil {
		if err := idGenerator(c, record); err != nil {
			return err
		}
	}
	return nil
}

func TestInserter(t *testing.T) {
	var inserter Inserter = inserterMock{}
	ctx := context.Background()
	var record Record

	suppliedKey := NewRecordKey(RecordRef{Kind: "foo", ID: ""})
	record = NewRecord(suppliedKey, foo{title: ""})

	defer func() {
		_ = recover()
	}()
	err := inserter.Insert(ctx, record, NewInsertOptions(WithRandomStringID(5)))
	if err != nil {
		t.Error(err)
	}
	recordKey := record.Key()
	if len(recordKey) != len(suppliedKey) {
		t.Errorf("len(recordKey) != (suppliedKey): %v != %v", recordKey, suppliedKey)
	}
	if id := recordKey[0].ID; len(id.(string)) != 5 {
		t.Errorf("len(recordKey[0].ID) expected to be 5, got: %v: %v", len(id.(string)), id)
	}
}

func TestInsertWithRandomID(t *testing.T) {
	t.Run("should_pass", func(t *testing.T) {
		data := new(o)

		generatesCount := 0
		var generateID IDGenerator
		generateID = func(ctx context.Context, record Record) error {
			generatesCount++
			return nil
		}

		exists := func(key RecordKey) error {
			if generatesCount < 3 {
				return nil
			}
			return ErrRecordNotFound
		}
		insertsCount := 0
		insert := func(r Record) error {
			insertsCount++
			return nil
		}
		err := InsertWithRandomID(nil, record{data: data}, generateID,
			5,
			exists, insert)
		if err != nil {
			t.Errorf("failed to insert: %v", err)
		}
		if generatesCount != 3 {
			t.Errorf("ID generator expected to be called 3 times, actual: %v", generatesCount)
		}
	})
}
