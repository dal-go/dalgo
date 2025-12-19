package record

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/update"
	"github.com/stretchr/testify/assert"
)

type fakeRecord struct {
	key  *dal.Key
	data any
	err  error
}

func (f *fakeRecord) Key() *dal.Key { return f.key }
func (f *fakeRecord) Error() error  { return nil }
func (f *fakeRecord) Exists() bool  { return false }
func (f *fakeRecord) SetError(err error) dal.Record {
	f.err = err
	return f
}
func (f *fakeRecord) Data() any        { return f.data }
func (f *fakeRecord) HasChanged() bool { return false }
func (f *fakeRecord) MarkAsChanged()   {}

func TestWithRecordChanges_ApplyChanges(t *testing.T) {
	type fields struct {
		recordsToInsert []dal.Record
		RecordsToUpdate []*Updates
		RecordsToDelete []*dal.Key
	}
	type args struct {
		ctx         context.Context
		tx          dal.ReadwriteTransaction
		excludeKeys []*dal.Key
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		assertErr assert.ErrorAssertionFunc
	}{
		{
			name: "nil",
			fields: fields{
				recordsToInsert: nil,
				RecordsToUpdate: nil,
				RecordsToDelete: nil,
			},
			assertErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &WithRecordChanges{
				recordsToInsert: tt.fields.recordsToInsert,
				RecordsToUpdate: tt.fields.RecordsToUpdate,
				RecordsToDelete: tt.fields.RecordsToDelete,
			}
			err := v.ApplyChanges(tt.args.ctx, tt.args.tx, tt.args.excludeKeys...)
			tt.assertErr(t, err)
		})
	}
}

func TestWithRecordChanges_QueueForInsert(t *testing.T) {
	type fields struct {
		recordsToInsert []dal.Record
	}
	type args struct {
		records []dal.Record
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "nil",
			fields: fields{
				recordsToInsert: nil,
			},
			args: args{
				records: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &WithRecordChanges{
				recordsToInsert: tt.fields.recordsToInsert,
			}
			v.QueueForInsert(tt.args.records...)
		})
	}
}

func TestWithRecordChanges_RecordsToInsert(t *testing.T) {
	t.Run("empty_slice", func(t *testing.T) {
		v := &WithRecordChanges{recordsToInsert: []dal.Record{}}
		result := v.RecordsToInsert()
		assert.Equal(t, []dal.Record{}, result)
	})

	t.Run("nil_slice", func(t *testing.T) {
		v := &WithRecordChanges{recordsToInsert: nil}
		result := v.RecordsToInsert()
		assert.Nil(t, result)
	})

	t.Run("with_records_returns_copy", func(t *testing.T) {
		key := dal.NewKeyWithID("test", "id1")
		record := dal.NewRecordWithData(key, map[string]any{"test": "data"})
		v := &WithRecordChanges{recordsToInsert: []dal.Record{record}}

		result := v.RecordsToInsert()
		assert.Equal(t, 1, len(result))
		assert.Equal(t, record, result[0])

		// Verify it's a copy by modifying original slice
		v.recordsToInsert[0] = nil
		result2 := v.RecordsToInsert()
		assert.NotEqual(t, result[0], result2[0])
	})
}

func TestWithRecordChanges_QueueForInsert_Panics(t *testing.T) {
	t.Run("panic_on_nil_record", func(t *testing.T) {
		v := &WithRecordChanges{}
		assert.PanicsWithValue(t, "record #0 is required", func() {
			v.QueueForInsert(nil)
		})
	})

	t.Run("panic_on_nil_key", func(t *testing.T) {
		v := &WithRecordChanges{}
		record := &fakeRecord{key: nil, data: map[string]any{"x": 1}}
		assert.PanicsWithValue(t, "record #0.Key() is required", func() {
			v.QueueForInsert(record)
		})
	})

	t.Run("panic_on_nil_data", func(t *testing.T) {
		v := &WithRecordChanges{}
		key := dal.NewKeyWithID("test", "id1")
		record := &fakeRecord{key: key, data: nil}
		assert.PanicsWithValue(t, "record #0.Data() is required", func() {
			v.QueueForInsert(record)
		})
	})

	t.Run("panic_on_duplicate_key", func(t *testing.T) {
		v := &WithRecordChanges{}
		key := dal.NewKeyWithID("test", "id1")
		record1 := dal.NewRecordWithData(key, map[string]any{"test": "data1"})
		record2 := dal.NewRecordWithData(key, map[string]any{"test": "data2"})
		// Ensure records simulate a post-Get state
		record1.SetError(nil)
		record2.SetError(nil)

		v.QueueForInsert(record1)
		assert.PanicsWithValue(t, "record with key=test/id1 is already queued for insert", func() {
			v.QueueForInsert(record2)
		})
	})

	t.Run("successful_queue", func(t *testing.T) {
		v := &WithRecordChanges{}
		key1 := dal.NewKeyWithID("test", "id1")
		key2 := dal.NewKeyWithID("test", "id2")
		record1 := dal.NewRecordWithData(key1, map[string]any{"test": "data1"})
		record2 := dal.NewRecordWithData(key2, map[string]any{"test": "data2"})
		// Simulate records being ready for use
		record1.SetError(nil)
		record2.SetError(nil)

		v.QueueForInsert(record1, record2)
		assert.Equal(t, 2, len(v.recordsToInsert))
	})
}

func Test_excludeRecords(t *testing.T) {
	t.Run("no_exclude_keys", func(t *testing.T) {
		key1 := dal.NewKeyWithID("test", "id1")
		record1 := dal.NewRecordWithData(key1, map[string]any{"test": "data1"})
		records := []dal.Record{record1}

		result := excludeRecords(records, nil)
		assert.Equal(t, records, result)
	})

	t.Run("exclude_some_records", func(t *testing.T) {
		key1 := dal.NewKeyWithID("test", "id1")
		key2 := dal.NewKeyWithID("test", "id2")
		key3 := dal.NewKeyWithID("test", "id3")
		record1 := dal.NewRecordWithData(key1, map[string]any{"test": "data1"})
		record2 := dal.NewRecordWithData(key2, map[string]any{"test": "data2"})
		record3 := dal.NewRecordWithData(key3, map[string]any{"test": "data3"})
		records := []dal.Record{record1, record2, record3}

		result := excludeRecords(records, []*dal.Key{key2})
		assert.Equal(t, 2, len(result))
		assert.Equal(t, record1, result[0])
		assert.Equal(t, record3, result[1])
	})

	t.Run("exclude_all_records", func(t *testing.T) {
		key1 := dal.NewKeyWithID("test", "id1")
		record1 := dal.NewRecordWithData(key1, map[string]any{"test": "data1"})
		records := []dal.Record{record1}

		result := excludeRecords(records, []*dal.Key{key1})
		assert.Equal(t, 0, len(result))
	})
}

// fakeTx implements dal.ReadwriteTransaction with minimal behavior needed for tests
type fakeTx struct {
	insertErr error
	updateErr error
	deleteErr error

	inserted []dal.Record
	updated  []struct {
		key     *dal.Key
		updates []update.Update
	}
	deleted []*dal.Key
}

var _ dal.ReadwriteTransaction = (*fakeTx)(nil)

// Transaction + ReadwriteTransaction
func (f *fakeTx) ID() string                      { return "fake-tx" }
func (f *fakeTx) Options() dal.TransactionOptions { return dal.NewTransactionOptions() }
func (f *fakeTx) Name() string                    { return "fake" }

// ReadSession (Getter, MultiGetter, QueryExecutor)
func (f *fakeTx) Get(_ context.Context, _ dal.Record) error          { return nil }
func (f *fakeTx) Exists(_ context.Context, _ *dal.Key) (bool, error) { return false, nil }
func (f *fakeTx) GetMulti(_ context.Context, _ []dal.Record) error   { return nil }
func (f *fakeTx) GetReader(_ context.Context, _ dal.Query) (dal.Reader, error) {
	return nil, nil
}
func (f *fakeTx) ReadAllRecords(_ context.Context, _ dal.Query, _ ...dal.ReaderOption) ([]dal.Record, error) {
	return nil, nil
}

// WriteSession (Setter, MultiSetter, Deleter, MultiDeleter, Updater, MultiUpdater, Inserter, MultiInserter)
func (f *fakeTx) Set(_ context.Context, _ dal.Record) error        { return nil }
func (f *fakeTx) SetMulti(_ context.Context, _ []dal.Record) error { return nil }
func (f *fakeTx) Delete(_ context.Context, _ *dal.Key) error       { return nil }
func (f *fakeTx) DeleteMulti(_ context.Context, keys []*dal.Key) error {
	f.deleted = append(f.deleted, keys...)
	return f.deleteErr
}
func (f *fakeTx) Update(_ context.Context, key *dal.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	_ = preconditions
	f.updated = append(f.updated, struct {
		key     *dal.Key
		updates []update.Update
	}{key: key, updates: updates})
	return f.updateErr
}
func (f *fakeTx) UpdateRecord(_ context.Context, _ dal.Record, _ []update.Update, _ ...dal.Precondition) error {
	return f.updateErr
}
func (f *fakeTx) UpdateMulti(_ context.Context, _ []*dal.Key, _ []update.Update, _ ...dal.Precondition) error {
	return nil
}
func (f *fakeTx) Insert(_ context.Context, record dal.Record, _ ...dal.InsertOption) error {
	f.inserted = append(f.inserted, record)
	return f.insertErr
}
func (f *fakeTx) InsertMulti(_ context.Context, records []dal.Record, _ ...dal.InsertOption) error {
	f.inserted = append(f.inserted, records...)
	return f.insertErr
}

func TestWithRecordChanges_ApplyChanges_SuccessPaths(t *testing.T) {
	ctx := context.Background()
	key1 := dal.NewKeyWithID("test", "id1")
	key2 := dal.NewKeyWithID("test", "id2")
	rec1 := dal.NewRecordWithData(key1, map[string]any{"a": 1})
	rec1.SetError(nil)
	rec2 := dal.NewRecordWithData(key2, map[string]any{"b": 2})
	rec2.SetError(nil)

	wr := &WithRecordChanges{
		recordsToInsert: []dal.Record{rec1, rec2},
		RecordsToUpdate: []*Updates{
			{Record: rec1, Updates: []update.Update{update.ByFieldName("a", 10)}},
		},
		RecordsToDelete: []*dal.Key{key2},
	}
	tx := &fakeTx{}
	err := wr.ApplyChanges(ctx, tx)
	assert.NoError(t, err)
	// Inserted all
	assert.Equal(t, 2, len(tx.inserted))
	// Updated
	if assert.Equal(t, 1, len(tx.updated)) {
		assert.Equal(t, key1, tx.updated[0].key)
		assert.Equal(t, 1, len(tx.updated[0].updates))
	}
	// Deleted
	assert.Equal(t, []*dal.Key{key2}, tx.deleted)
	// State reset
	assert.Nil(t, wr.recordsToInsert)
	assert.Nil(t, wr.RecordsToUpdate)
	assert.Nil(t, wr.RecordsToDelete)
}

func TestWithRecordChanges_ApplyChanges_InsertError(t *testing.T) {
	ctx := context.Background()
	key := dal.NewKeyWithID("test", "id1")
	rec := dal.NewRecordWithData(key, map[string]any{"a": 1})
	rec.SetError(nil)
	wr := &WithRecordChanges{recordsToInsert: []dal.Record{rec}}
	tx := &fakeTx{insertErr: errors.New("boom")}
	err := wr.ApplyChanges(ctx, tx)
	assert.EqualError(t, err, "failed to insert records: boom")
}

func TestWithRecordChanges_ApplyChanges_UpdateError(t *testing.T) {
	ctx := context.Background()
	key := dal.NewKeyWithID("test", "id1")
	rec := dal.NewRecordWithData(key, map[string]any{"a": 1})
	rec.SetError(nil)
	wr := &WithRecordChanges{RecordsToUpdate: []*Updates{{Record: rec, Updates: []update.Update{update.ByFieldName("a", 2)}}}}
	tx := &fakeTx{updateErr: errors.New("upderr")}
	err := wr.ApplyChanges(ctx, tx)
	assert.NotNil(t, err)
	assert.Error(t, err)
	if err != nil {
		assert.Contains(t, err.Error(), "failed to update record test/id1: upderr")
	}
}

func TestWithRecordChanges_ApplyChanges_DeleteError(t *testing.T) {
	ctx := context.Background()
	key := dal.NewKeyWithID("test", "id1")
	wr := &WithRecordChanges{RecordsToDelete: []*dal.Key{key}}
	tx := &fakeTx{deleteErr: errors.New("delerr")}
	err := wr.ApplyChanges(ctx, tx)
	assert.EqualError(t, err, "failed to delete records: delerr")
}

func TestWithRecordChanges_ApplyChanges_ExcludeKeys(t *testing.T) {
	ctx := context.Background()
	key1 := dal.NewKeyWithID("test", "id1")
	key2 := dal.NewKeyWithID("test", "id2")
	rec1 := dal.NewRecordWithData(key1, map[string]any{"a": 1})
	rec1.SetError(nil)
	rec2 := dal.NewRecordWithData(key2, map[string]any{"b": 2})
	rec2.SetError(nil)
	wr := &WithRecordChanges{recordsToInsert: []dal.Record{rec1, rec2}}
	tx := &fakeTx{}
	err := wr.ApplyChanges(ctx, tx, key1)
	assert.NoError(t, err)
	assert.Equal(t, []dal.Record{rec2}, tx.inserted)
}
