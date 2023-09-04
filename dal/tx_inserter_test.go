package dal

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"strconv"
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
//func (v inserterMock) Insert(ctx context.Context, record Record, opts ...InsertOption) error {
//	options := NewInsertOptions(opts...)
//	if idGenerator := options.IDGenerator(); idGenerator != nil {
//		if err := idGenerator(ctx, record); err != nil {
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
}

func TestInsertWithRandomID(t *testing.T) {

	for _, tt := range []struct {
		name string
		args insertArgs
		//
		generatorErr            error
		generatorErrOnAttemptNo int
		expectedErrTexts        []string
		existsFalseOnAttemptNo  int
		existsErrorOnAttemptNo  int
		existsError             error
	}{
		{
			name:                   "should_pass_on_first_attempt",
			existsFalseOnAttemptNo: 1,
			args: insertArgs{
				ctx:      context.Background(),
				record:   NewRecordWithData(&Key{collection: "test_kind"}, new(map[string]any)),
				attempts: 1,
			},
		},
		{
			name:                   "exists_error_on_first_attempt",
			existsErrorOnAttemptNo: 1,
			existsError:            errors.New("test exists error"),
			existsFalseOnAttemptNo: 1,
			args: insertArgs{
				ctx:      context.Background(),
				record:   NewRecordWithData(&Key{collection: "test_kind"}, new(map[string]any)),
				attempts: 1,
			},
		},
		{
			name:                   "exceeds_max_generates_count",
			existsFalseOnAttemptNo: 7,
			args: insertArgs{
				ctx:      context.Background(),
				record:   NewRecordWithData(&Key{collection: "test_kind"}, new(map[string]any)),
				attempts: 5,
			},
			generatorErr:     ErrExceedsMaxNumberOfAttempts,
			expectedErrTexts: []string{ErrExceedsMaxNumberOfAttempts.Error(), "5"},
		},
		{
			name:                    "generator_err_on_first_attempt",
			generatorErrOnAttemptNo: 1,
			generatorErr:            errors.New("test generator intentional error"),
			args: insertArgs{
				ctx:      context.Background(),
				record:   NewRecordWithData(&Key{collection: "test_kind"}, new(map[string]any)),
				attempts: 5,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {

			attempt := 0
			var generateID = func(ctx context.Context, record Record) error {
				attempt++
				if tt.generatorErrOnAttemptNo == attempt {
					return tt.generatorErr
				}
				record.Key().ID = strconv.Itoa(attempt)
				return nil
			}

			exists := func(key *Key) error {
				if tt.existsErrorOnAttemptNo == attempt {
					return tt.existsError
				}
				if attempt < tt.existsFalseOnAttemptNo {
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
				if tt.generatorErr != nil && !errors.Is(err, tt.generatorErr) {
					t.Errorf("expected error: %v, actual: %v", tt.generatorErr, err)
				}
				if tt.existsError != nil && !errors.Is(err, tt.existsError) {
					t.Errorf("expected error: %v, actual: %v", tt.existsError, err)
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
			assert.Equal(t, 1, insertsCount)
			if attempt != tt.existsFalseOnAttemptNo {
				t.Errorf("id generator expected to be called %d times, actual: %d", tt.existsFalseOnAttemptNo, attempt)
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
