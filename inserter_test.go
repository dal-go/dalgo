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

func (v inserterMock) Insert(c context.Context, record Record, opts ...InsertOption) error {
	options := NewInsertOptions(opts...)
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

	suppliedKey := NewKey("foo", WithRandomStringID(5))
	record = NewRecord(suppliedKey)

	defer func() {
		_ = recover()
	}()
	err := inserter.Insert(ctx, record)
	if err != nil {
		t.Error(err)
	}
	recordKey := record.Key()
	if id := recordKey.ID; len(id.(string)) != 5 {
		t.Errorf("len(recordKey[0].Value) expected to be 5, got: %v: %v", len(id.(string)), id)
	}
}

func TestInsertWithRandomID(t *testing.T) {
	t.Run("should_pass", func(t *testing.T) {
		data := new(map[string]interface{})

		generatesCount := 0
		var generateID IDGenerator
		generateID = func(ctx context.Context, record Record) error {
			generatesCount++
			return nil
		}

		exists := func(key *Key) error {
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
		err := InsertWithRandomID(context.Background(), NewRecordWithData(&Key{kind: "test_kind"}, data), generateID,
			5,
			exists, insert)
		if err != nil {
			t.Errorf("failed to insert: %v", err)
		}
		if generatesCount != 3 {
			t.Errorf("Value generator expected to be called 3 times, actual: %v", generatesCount)
		}
	})
}
