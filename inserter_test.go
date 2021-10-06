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

func (v inserterMock) Insert(c context.Context, record Record, opts ...insertOption) error {
	options := NewInsertOptions(opts...)
	if idGenerator := options.IDGenerator(); idGenerator != nil {
		if err := idGenerator(c, record); err != nil {
			return err
		}
	}
	return nil
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
