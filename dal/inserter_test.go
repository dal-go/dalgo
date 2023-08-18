package dal

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

//type foo struct {
//	title string
//}
//
//func (foo foo) Kind() string {
//	return "foo"
//}
//
//func (foo foo) Validate() error {
//	if foo.title == "" {
//		return fmt.Errorf("missing required field: title")
//	}
//	return nil
//}

//type inserterMock struct {
//}
//
//func (v inserterMock) Insert(c context.Context, record Record, opts ...InsertOption) error {
//	options := NewInsertOptions(opts...)
//	if idGenerator := options.IDGenerator(); idGenerator != nil {
//		if err := idGenerator(c, record); err != nil {
//			return err
//		}
//	}
//	return nil
//}

func TestInsertWithRandomID(t *testing.T) {
	t.Run("should_pass", func(t *testing.T) {
		data := new(map[string]any)

		generatesCount := 0
		var generateID = func(ctx context.Context, record Record) error {
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
		err := InsertWithRandomID(context.Background(), NewRecordWithData(&Key{collection: "test_kind"}, data), generateID,
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

func TestInsertOptions_IDGenerator(t *testing.T) {
	err := errors.New("test error")
	idGenerator := func(ctx context.Context, record Record) error {
		return err
	}
	io := insertOptions{
		idGenerator: idGenerator,
	}
	assert.Equal(t, err, io.IDGenerator()(context.Background(), nil))
}

func TestNewInsertOptions(t *testing.T) {
	called := false
	o := func(options *insertOptions) {
		called = true
	}
	io := NewInsertOptions(o)
	assert.NotNil(t, io)
	assert.True(t, called)
}

func TestWithRandomStringID(t *testing.T) {
	key, err := NewKeyWithOptions("c1", WithRandomStringID(context.Background(), 10))
	assert.Nil(t, err)
	assert.NotNil(t, key)
	id := key.ID.(string)
	assert.NotEqual(t, "", id)
	assert.Equal(t, 10, len(id))
}

func TestWithPrefix(t *testing.T) {
	key, err := NewKeyWithOptions("c1", WithRandomStringID(context.Background(), 10, WithPrefix("prefix_")))
	assert.Nil(t, err)
	assert.NotNil(t, key)
	assert.True(t, strings.HasPrefix(key.ID.(string), "prefix_"))
}
