package dalgo2memory

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/dalgo/update"
)

// NewDB creates an in-memory DALgo database.
func NewDB(options ...Option) dal.DB {
	db := &database{
		collections: make(map[string]storageEngine),
	}
	for _, option := range options {
		if option != nil {
			option(db)
		}
	}
	return db
}

type database struct {
	dal.ConcurrencyAvailable

	mu sync.RWMutex
	// collections maps a collection name to its storage engine. Stored records
	// live solely inside these engine instances; the engine is created lazily
	// on first access (see database.engine).
	collections map[string]storageEngine
	schema      *memorySchema
}

func (db *database) ID() string {
	return "dalgo2memory"
}

func (db *database) Adapter() dal.Adapter {
	return dal.NewAdapter("memory", "0.0.1")
}

func (db *database) Schema() dal.Schema {
	return nil
}

func (db *database) RunReadonlyTransaction(ctx context.Context, f dal.ROTxWorker, _ ...dal.TransactionOption) error {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return f(ctx, session{db: db})
}

func (db *database) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, _ ...dal.TransactionOption) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return f(ctx, session{db: db})
}

func (db *database) Exists(ctx context.Context, key *dal.Key) (bool, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return session{db: db}.Exists(ctx, key)
}

func (db *database) Get(ctx context.Context, record dal.Record) error {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return session{db: db}.Get(ctx, record)
}

func (db *database) GetMulti(ctx context.Context, records []dal.Record) error {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return session{db: db}.GetMulti(ctx, records)
}

func (db *database) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return session{db: db}.ExecuteQueryToRecordsReader(ctx, query)
}

func (db *database) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return session{db: db}.ExecuteQueryToRecordsetReader(ctx, query, options...)
}

func (db *database) Set(ctx context.Context, record dal.Record) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return session{db: db}.Set(ctx, record)
}

func (db *database) SetMulti(ctx context.Context, records []dal.Record) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return session{db: db}.SetMulti(ctx, records)
}

func (db *database) Insert(ctx context.Context, record dal.Record, opts ...dal.InsertOption) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return session{db: db}.Insert(ctx, record, opts...)
}

func (db *database) InsertMulti(ctx context.Context, records []dal.Record, opts ...dal.InsertOption) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return session{db: db}.InsertMulti(ctx, records, opts...)
}

func (db *database) Delete(ctx context.Context, key *dal.Key) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return session{db: db}.Delete(ctx, key)
}

func (db *database) DeleteMulti(ctx context.Context, keys []*dal.Key) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return session{db: db}.DeleteMulti(ctx, keys)
}

func (db *database) Update(ctx context.Context, key *dal.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return session{db: db}.Update(ctx, key, updates, preconditions...)
}

func (db *database) UpdateRecord(ctx context.Context, record dal.Record, updates []update.Update, preconditions ...dal.Precondition) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return session{db: db}.UpdateRecord(ctx, record, updates, preconditions...)
}

func (db *database) UpdateMulti(ctx context.Context, keys []*dal.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return session{db: db}.UpdateMulti(ctx, keys, updates, preconditions...)
}

var _ dal.DB = (*database)(nil)

type session struct {
	db *database
}

func (s session) ID() string {
	return ""
}

func (s session) Options() dal.TransactionOptions {
	return nil
}

func (s session) Exists(_ context.Context, key *dal.Key) (bool, error) {
	return s.db.engine(key.Collection()).exists(keyID(key)), nil
}

func (s session) Get(_ context.Context, record dal.Record) error {
	if err := s.db.guardCollection(record.Key().Collection()); err != nil {
		record.SetError(err)
		return err
	}
	record.SetError(nil)
	if err := s.db.engine(record.Key().Collection()).load(keyID(record.Key()), record); err != nil {
		record.SetError(err)
		return err
	}
	return nil
}

func (s session) GetMulti(ctx context.Context, records []dal.Record) error {
	for _, record := range records {
		if err := s.Get(ctx, record); err != nil && !dal.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (s session) Set(_ context.Context, record dal.Record) error {
	return s.save(record, true)
}

func (s session) SetMulti(ctx context.Context, records []dal.Record) error {
	for _, record := range records {
		if err := s.Set(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (s session) Insert(_ context.Context, record dal.Record, _ ...dal.InsertOption) error {
	return s.save(record, false)
}

func (s session) InsertMulti(ctx context.Context, records []dal.Record, opts ...dal.InsertOption) error {
	for _, record := range records {
		if err := s.Insert(ctx, record, opts...); err != nil {
			return err
		}
	}
	return nil
}

func (s session) Delete(_ context.Context, key *dal.Key) error {
	s.db.engine(key.Collection()).delete(keyID(key))
	return nil
}

func (s session) DeleteMulti(ctx context.Context, keys []*dal.Key) error {
	for _, key := range keys {
		_ = s.Delete(ctx, key)
	}
	return nil
}

func (s session) Update(ctx context.Context, key *dal.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	record := dal.NewRecordWithData(key, map[string]any{})
	return s.UpdateRecord(ctx, record, updates, preconditions...)
}

func (s session) UpdateRecord(_ context.Context, record dal.Record, updates []update.Update, _ ...dal.Precondition) error {
	collectionName := record.Key().Collection()
	if err := s.db.guardCollection(collectionName); err != nil {
		return err
	}
	fieldUpdates := make(map[string]any, len(updates))
	for _, upd := range updates {
		if fieldName := upd.FieldName(); fieldName != "" {
			fieldUpdates[fieldName] = upd.Value()
		}
	}
	return s.db.engine(collectionName).update(keyID(record.Key()), fieldUpdates)
}

func (s session) UpdateMulti(ctx context.Context, keys []*dal.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	for _, key := range keys {
		if err := s.Update(ctx, key, updates, preconditions...); err != nil {
			return err
		}
	}
	return nil
}

func (s session) ExecuteQueryToRecordsReader(_ context.Context, query dal.Query) (dal.RecordsReader, error) {
	q, ok := query.(dal.StructuredQuery)
	if !ok {
		return nil, dal.ErrNotSupported
	}
	if len(q.From().Joins()) > 0 {
		return s.executeJoinQuery(q)
	}
	base := q.From().Base()
	collectionName := base.Name()
	factory, err := s.db.recordFactory(collectionName)
	if err != nil {
		return nil, err
	}
	allRows, err := s.loadRows(collectionName)
	if err != nil {
		return nil, err
	}
	if len(allRows) == 0 {
		return dal.NewRecordsReader([]dal.Record{}), nil
	}
	known := map[string]bool{"": true, base.Name(): true}
	if a := base.Alias(); a != "" {
		known[a] = true
	}
	if err := validateOrderSources(q.OrderBy(), known); err != nil {
		return nil, err
	}
	grouped := len(q.GroupBy()) > 0
	if !grouped {
		if err := validateColumns(q.Columns(), known); err != nil {
			return nil, err
		}
	}
	rows := make([]memoryRow, 0, len(allRows))
	for _, row := range allRows {
		if !matchesWhere(row.data, q.Where()) {
			continue
		}
		rows = append(rows, row)
	}
	if grouped {
		srcs := make([]rowSources, len(rows))
		for i, row := range rows {
			srcs[i] = baseSources(base, row.data)
		}
		return executeGroupedReader(q, srcs, collectionName, known)
	}
	orderBySources(rows, q.OrderBy(),
		func(r memoryRow) map[string]map[string]any {
			return baseSources(base, r.data)
		},
		func(r memoryRow) string { return r.id })
	if limit := q.Limit(); limit > 0 && limit < len(rows) {
		rows = rows[:limit]
	}
	columns := q.Columns()
	records := make([]dal.Record, len(rows))
	for i, row := range rows {
		key := dal.NewKeyWithID(collectionName, row.id)
		if len(columns) > 0 {
			records[i] = dal.NewRecordWithData(key, projectRow(columns, baseSources(base, row.data))).SetError(nil)
			continue
		}
		template := q.IntoRecord()
		if template == nil && factory != nil {
			data := factory()
			if err := row.materialize(data); err != nil {
				return nil, err
			}
			records[i] = dal.NewRecordWithData(key, data).SetError(nil)
			continue
		}
		if template == nil {
			records[i] = dal.NewRecord(key).SetError(nil)
			continue
		}
		data := template.Data()
		if err := row.materialize(data); err != nil {
			return nil, err
		}
		records[i] = dal.NewRecordWithData(key, data).SetError(nil)
	}
	return dal.NewRecordsReader(records), nil
}

var _ dal.ReadwriteTransaction = (*session)(nil)

type memoryRow struct {
	id          string
	data        map[string]any
	materialize func(target any) error
}

func (s session) save(record dal.Record, overwrite bool) error {
	collectionName := record.Key().Collection()
	if err := s.db.guardCollection(collectionName); err != nil {
		record.SetError(err)
		return err
	}
	record.SetError(nil)
	if err := s.db.engine(collectionName).store(keyID(record.Key()), record, overwrite); err != nil {
		record.SetError(err)
		return err
	}
	record.SetError(nil)
	return nil
}

func keyID(key *dal.Key) string {
	return fmt.Sprint(key.ID)
}

func matchesWhere(data map[string]any, condition dal.Condition) bool {
	if condition == nil {
		return true
	}
	comparison, ok := condition.(dal.Comparison)
	if !ok || comparison.Operator != dal.Equal {
		return false
	}
	field, ok := comparison.Left.(dal.FieldRef)
	if !ok {
		return false
	}
	constant, ok := comparison.Right.(dal.Constant)
	if !ok {
		return false
	}
	return data[field.Name()] == constant.Value
}

func compare(a, b any) int {
	af, aOK := number(a)
	bf, bOK := number(b)
	if aOK && bOK {
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		default:
			return 0
		}
	}
	as, bs := fmt.Sprint(a), fmt.Sprint(b)
	switch {
	case as < bs:
		return -1
	case as > bs:
		return 1
	default:
		return 0
	}
}

func number(v any) (float64, bool) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return 0, false
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint()), true
	case reflect.Float32, reflect.Float64:
		return rv.Float(), true
	default:
		return 0, false
	}
}

func isDuplicate(err error) bool {
	return err != nil && !errors.Is(err, dal.ErrRecordNotFound)
}
