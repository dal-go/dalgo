package dalgo2memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/dalgo/update"
)

// NewDB creates an in-memory DALgo database.
func NewDB(options ...Option) dal.DB {
	db := &database{
		collections:       make(map[string]storageEngine),
		schemaRefBreaking: true,
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
	// schemaRefBreaking is the schema-wide columnar fidelity default (faithful
	// unless WithoutSchemaRefBreaking was used). NewDB initializes it to true.
	schemaRefBreaking bool
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

// insertWithGeneratorMaxAttempts bounds the id-generation collision-retry loop
// when honoring an InsertOption generator. The deferred InsertOption family does
// not expose its own max-attempts through the InsertOptions interface, so the
// storage layer picks the bound; random-string collisions are astronomically
// rare, so a generous bound is effectively only hit by deterministic generators.
const insertWithGeneratorMaxAttempts = 100

func (s session) Insert(ctx context.Context, record dal.Record, opts ...dal.InsertOption) error {
	options := dal.NewInsertOptions(opts...)
	gen := options.IDGenerator()
	if gen == nil && options.PreferAdapterGeneratedID() {
		// The in-memory backend has no native ID generation mechanism, so per the
		// dal.WithAdapterGeneratedID contract fall back to the default random-string generator.
		gen = dal.NewInsertOptions(dal.WithRandomStringKey(dal.DefaultRandomStringIDLength, 5)).IDGenerator()
	}
	if gen != nil {
		return dal.InsertWithIdGenerator(ctx, record, gen, insertWithGeneratorMaxAttempts,
			func(key *dal.Key) error {
				if s.db.engine(key.Collection()).exists(keyID(key)) {
					return nil // id is taken: signal "exists" so generation retries
				}
				return dal.ErrRecordNotFound // id is free: signal "not found" so it is inserted
			},
			func(r dal.Record) error {
				return s.save(r, false)
			},
		)
	}
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
	return s.db.engine(collectionName).update(keyID(record.Key()), updates)
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
	allRows, err := s.loadCandidateRows(collectionName, q.Where())
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
	var parent *dal.Key
	if cr, ok := base.(dal.CollectionRef); ok {
		parent = cr.Parent()
	}
	rows := make([]memoryRow, 0, len(allRows))
	for _, row := range allRows {
		if !matchesWhere(row.data, q.Where()) {
			continue
		}
		// Parent-scoped query: keep only rows whose stored key is a direct child
		// of the requested parent. A nil parent (root collection ref) is the
		// collection-group case and keeps every row across all parents.
		if parent != nil && !isChildOf(row.key, parent) {
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
		key := resultKey(row, collectionName)
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
	key         *dal.Key
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

// keyID is a record's storage identity within its per-leaf-collection engine.
// Records are grouped by leaf collection name, so a nested record's leaf id is
// not unique on its own: spaces/A/items/i1 and spaces/B/items/i1 share the leaf
// id "i1" yet are distinct records (as in Firestore). Root-level records keep
// the bare leaf id (backward compatible, and what the white-box engine tests
// address records by); nested records are keyed by their full parent-chain path
// so the same leaf id under different parents no longer collides.
func keyID(key *dal.Key) string {
	if key.Parent() == nil {
		return fmt.Sprint(key.ID)
	}
	return keyPath(key)
}

// keyPath builds a record's full "collection/id/collection/id/…" path from the
// root down. Unlike dal.Key.String it performs no validation and so never
// panics on an incomplete key — keyID can run on the insert-generator path
// before a generated id is finalized.
func keyPath(key *dal.Key) string {
	var parts []string
	for k := key; k != nil; k = k.Parent() {
		parts = append([]string{k.Collection() + "/" + fmt.Sprint(k.ID)}, parts...)
	}
	return strings.Join(parts, "/")
}

// resultKey returns the stored full key (with its parent chain) for a query
// result row, falling back to a leaf-only key when a row carries none.
func resultKey(row memoryRow, collectionName string) *dal.Key {
	if row.key != nil {
		return row.key
	}
	return dal.NewKeyWithID(collectionName, row.id)
}

// isChildOf reports whether key's immediate parent is the given parent (by full
// path) — used to scope a parent-anchored collection query. Both keys come from
// stored records / collection refs and so are valid.
func isChildOf(key, parent *dal.Key) bool {
	if key == nil || parent == nil {
		return false
	}
	p := key.Parent()
	if p == nil {
		return false
	}
	return p.String() == parent.String()
}

// matchesWhere evaluates the WHERE condition shapes that dalgo2firestore
// translates to native Firestore filters, so memory-backed tests behave like
// the Firestore adapter:
//
//   - FieldRef op Constant for ==, >, >=, <, <=
//   - Constant In FieldRef    → Firestore's "array-contains"
//   - FieldRef op dal.Array   → Firestore's "array-contains-any"
//   - GroupCondition with AND → all sub-conditions must match
//
// Any other shape (including OR groups, which dalgo2firestore rejects) does
// not match.
func matchesWhere(data map[string]any, condition dal.Condition) bool {
	if condition == nil {
		return true
	}
	switch cond := condition.(type) {
	case dal.GroupCondition:
		if cond.Operator() != dal.And {
			return false
		}
		for _, c := range cond.Conditions() {
			if !matchesWhere(data, c) {
				return false
			}
		}
		return true
	case dal.Comparison:
		return matchesComparison(data, cond)
	default:
		return false
	}
}

func matchesComparison(data map[string]any, comparison dal.Comparison) bool {
	switch left := comparison.Left.(type) {
	case dal.FieldRef:
		switch right := comparison.Right.(type) {
		case dal.Constant:
			norm := normalizeConstant(right.Value)
			switch comparison.Operator {
			case dal.Equal:
				return data[left.Name()] == norm
			case dal.GreaterThen, dal.GreaterOrEqual, dal.LessThen, dal.LessOrEqual:
				value, ok := data[left.Name()]
				if !ok {
					return false
				}
				return compareOp(comparison.Operator, value, norm)
			default:
				return false
			}
		case dal.Array:
			// dalgo2firestore maps FieldRef vs dal.Array to "array-contains-any"
			// regardless of the operator; mirror that.
			return fieldContainsAny(data[left.Name()], right.Value)
		default:
			return false
		}
	case dal.Constant:
		// dalgo2firestore maps Constant In FieldRef to "array-contains".
		right, ok := comparison.Right.(dal.FieldRef)
		if !ok || comparison.Operator != dal.In {
			return false
		}
		return fieldContains(data[right.Name()], normalizeConstant(left.Value))
	default:
		return false
	}
}

// fieldContains reports whether the record field's value is a slice or array
// containing v. Serialized rows decode JSON arrays as []any, so elements are
// matched per elementEquals.
func fieldContains(fieldValue, v any) bool {
	rv := reflect.ValueOf(fieldValue)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
		return false
	}
	for i := 0; i < rv.Len(); i++ {
		if elementEquals(rv.Index(i).Interface(), v) {
			return true
		}
	}
	return false
}

// fieldContainsAny reports whether the record field's value is a slice or
// array containing at least one element of values ("array-contains-any").
func fieldContainsAny(fieldValue, values any) bool {
	rv := reflect.ValueOf(values)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
		return false
	}
	for i := 0; i < rv.Len(); i++ {
		if fieldContains(fieldValue, rv.Index(i).Interface()) {
			return true
		}
	}
	return false
}

// elementEquals matches an array element against a constant. Numbers are
// compared by value because serialized rows decode all JSON numbers as
// float64; everything else uses Go equality (uncomparable values never match).
func elementEquals(a, b any) bool {
	if af, ok := number(a); ok {
		bf, ok := number(b)
		return ok && af == bf
	}
	if t := reflect.TypeOf(a); t != nil && !t.Comparable() {
		return false
	}
	if t := reflect.TypeOf(b); t != nil && !t.Comparable() {
		return false
	}
	return a == b
}

// normalizeConstant normalizes a query constant the same way stored row values
// are normalized (JSON marshal then unmarshal into any). This ensures that
// time.Time constants (marshaled to RFC3339 strings) compare correctly against
// the strings stored in serialized rows.
func normalizeConstant(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	// b is valid JSON produced by Marshal above, so unmarshalling into any never
	// fails.
	var out any
	_ = json.Unmarshal(b, &out)
	return out
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
