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

type insertArgs struct {
	ctx    context.Context
	record Record
	//generateID IDGenerator
	attempts int
	exists   func(*Key) error
	insert   func(Record) error
	//
}

func TestInsertWithRandomID(t *testing.T) {

	for _, tt := range []struct {
		name string
		args insertArgs
		//
		generatedID         any
		generatorErr        error
		expectedErrTexts    []string
		generateOnAttemptNo int
	}{
		{
			name:                "should_pass_on_first_attempt",
			generateOnAttemptNo: 1,
			args: insertArgs{
				ctx:      context.Background(),
				record:   NewRecordWithData(&Key{collection: "test_kind"}, new(map[string]any)),
				attempts: 1,
			},
			generatedID: "id1",
		},
		{
			name:                "exceeds_max_generates_count",
			generateOnAttemptNo: 7,
			args: insertArgs{
				ctx:      context.Background(),
				record:   NewRecordWithData(&Key{collection: "test_kind"}, new(map[string]any)),
				attempts: 5,
			},
			generatorErr:     ErrExceedsMaxNumberOfAttempts,
			expectedErrTexts: []string{ErrExceedsMaxNumberOfAttempts.Error(), "5"},
		},
		{
			name:                "should_fail",
			generatorErr:        errors.New("test generator intentional error"),
			generateOnAttemptNo: 2,
			args: insertArgs{
				ctx:      context.Background(),
				record:   NewRecordWithData(&Key{collection: "test_kind"}, new(map[string]any)),
				attempts: 5,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {

			generatesCount := 0
			var generateID = func(ctx context.Context, record Record) error {
				generatesCount++
				if tt.generateOnAttemptNo == generatesCount {
					record.Key().ID = tt.generatedID
					return tt.generatorErr
				}
				return nil
			}

			exists := func(key *Key) error {
				if generatesCount < tt.generateOnAttemptNo {
					return nil
				}
				return ErrRecordNotFound
			}

			insertsCount := 0
			insert := func(r Record) error {
				insertsCount++
				return nil
			}

			args := tt.args
			err := InsertWithRandomID(args.ctx, args.record, generateID, 5, exists, insert)
			if err != nil {
				if tt.generatorErr == nil {
					t.Errorf("failed to insert: %v", err)
				} else if !errors.Is(err, tt.generatorErr) {
					t.Errorf("expected error: %v, actual: %v", tt.generatorErr, err)
				}
				if len(tt.expectedErrTexts) > 0 {
					for _, expectedErrText := range tt.expectedErrTexts {
						if !strings.Contains(err.Error(), expectedErrText) {
							t.Errorf("expected error text to contain: %v, actual: %v", expectedErrText, err.Error())
						}
					}
				}
				assert.Nil(t, args.record.Key().ID)
				return
			}
			assert.NotNil(t, args.record.Key().ID)
			if generatesCount != tt.generateOnAttemptNo {
				t.Errorf("Value generator expected to be called 3 times, actual: %v", generatesCount)
			}
		})
	}
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
	const length = 10
	key, err := NewKeyWithOptions("c1", WithRandomStringID(RandomLength(length)))
	assert.Nil(t, err)
	assert.NotNil(t, key)
	id := key.ID.(string)
	assert.NotEqual(t, "", id)
	assert.Equal(t, length, len(id))
}

func TestWithPrefix(t *testing.T) {
	key, err := NewKeyWithOptions("c1", WithRandomStringID(Prefix("prefix_")))
	assert.Nil(t, err)
	assert.NotNil(t, key)
	assert.True(t, strings.HasPrefix(key.ID.(string), "prefix_"))
}

func TestWithIDGenerator(t *testing.T) {

	for _, tt := range []struct {
		name        string
		key         *Key
		id          string
		shouldPanic bool
	}{
		{name: "nil_id", shouldPanic: false, id: "id1", key: &Key{ID: nil, collection: "c1"}},
		{name: "nil_generator", shouldPanic: true, id: "", key: &Key{ID: "id1", collection: "c1"}},
		{name: "not_nil_id", shouldPanic: true, id: "id2", key: &Key{ID: "id1", collection: "c1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("panic expected")
					}
				}()
			}
			ctx := context.Background()
			var idGenerator IDGenerator
			if tt.id != "" {
				idGenerator = func(ctx context.Context, record Record) error {
					record.Key().ID = tt.id
					return nil
				}
			}
			err := WithIDGenerator(ctx, idGenerator)(tt.key)
			assert.Nil(t, err)
			assert.NotNil(t, tt.key.ID)
		})
	}
}
