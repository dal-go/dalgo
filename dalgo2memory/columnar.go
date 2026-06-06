package dalgo2memory

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/dal-go/dalgo/dal"
)

// compactionThreshold is the dead-slot fraction that triggers compaction. When
// dead/(live+dead) reaches this fraction (and at least one slot exists),
// maybeCompact reclaims the dead slots under the global write lock.
const compactionThreshold = 0.5

// column is one column of a columnar collection: the JSON field name it stores,
// a slice typed to the field's Go element type (or []any for an interface /
// heterogeneous field), a flag marking reference-bearing element types (which
// are deep-copied on write when faithful), the strategy backing it, and a back
// reference to the engine for liveness checks.
type column struct {
	name        string        // JSON object key (the field view / query key)
	goField     string        // the Go struct field name (for opt-out direct copy)
	values      reflect.Value // a slice of elemType, indexed by slot
	elemType    reflect.Type
	refBearing  bool
	strategy    ColumnStrategy
	defaultStgy *typedSliceStrategy // non-nil when strategy is the default
	engine      *columnarEngine
}

// columnarEngine stores a typed collection column-wise over a stable per-row
// slot model with tombstone deletes and bulk compaction. It implements the
// storageEngine seam and is selected per collection via WithColumnarStorage.
//
// All access is serialized by the database's single global write lock (db.mu),
// so the engine itself holds no lock.
type columnarEngine struct {
	collection string
	factory    func() any
	recordType reflect.Type // the struct type T (factory returns *T)

	columns   []*column
	byName    map[string]*column
	idToSlot  map[string]int
	slotToID  []string                  // slotToID[slot] == "" when the slot is dead
	live      []bool                    // live[slot] reports whether the slot holds a record
	freeList  []int                     // dead slots available for reuse
	deadCount int                       // number of dead (tombstoned) slots
	refBreak  bool                      // faithful (deep-copy ref-bearing columns) when true
	stgyByCol map[string]ColumnStrategy // explicit per-column strategies

	// mapBacked reports a mixed-mode map[string]any collection: columns come
	// from declared options and undeclared fields are kept in leftover.
	mapBacked bool
	// leftover holds each row's undeclared fields, indexed by the same per-row
	// slot as the column slices (nil for the struct path).
	leftover []map[string]any

	// initErr is set when the factory could not build a typed columnar engine
	// (schemaless/non-struct selection); every operation reports it.
	initErr error
}

var _ storageEngine = (*columnarEngine)(nil)

// columnarConfig is the resolved configuration produced by WithColumnarStorage:
// the per-column strategies, the optional per-collection ref-breaking override
// (nil means "inherit the schema-wide default"), and the columns explicitly
// declared for a map-backed (map[string]any) collection.
type columnarConfig struct {
	strategies   map[string]ColumnStrategy
	refBreakOver *bool
	declared     []declaredColumn
}

// declaredColumn names a column explicitly declared (via WithDeclaredColumn) for
// a map-backed columnar collection, together with the Go element type its typed
// slice holds. Declared columns are the column set for a map[string]any
// collection; on a struct collection they are accepted but redundant (the struct
// path reflects over the record type instead).
type declaredColumn struct {
	name     string
	elemType reflect.Type
}

// newColumnarEngineFactory builds an engineFactory that constructs a columnar
// engine for a typed collection. Selecting columnar storage for a schemaless or
// non-struct collection yields an engine whose operations all fail with a
// descriptive error (REQ:columnar-selection).
func newColumnarEngineFactory(cfg columnarConfig) engineFactory {
	return func(collection string, factory func() any, schemaRefBreaking bool) storageEngine {
		refBreak := schemaRefBreaking
		if cfg.refBreakOver != nil {
			refBreak = *cfg.refBreakOver
		}
		recordType, err := structTypeOf(factory)
		if err != nil {
			if isMapBackedFactory(factory) {
				return newMapBackedEngine(collection, factory, refBreak, cfg)
			}
			return &columnarEngine{
				collection: collection,
				factory:    factory,
				initErr:    fmt.Errorf("columnar storage for collection %q requires a schema-registered typed collection: %w", collection, err),
			}
		}
		eng := &columnarEngine{
			collection: collection,
			factory:    factory,
			recordType: recordType,
			byName:     make(map[string]*column),
			idToSlot:   make(map[string]int),
			refBreak:   refBreak,
			stgyByCol:  cfg.strategies,
		}
		eng.buildColumns()
		return eng
	}
}

// isMapBackedFactory reports whether the collection's record type is
// map[string]any (a schemaless mixed-mode collection), as opposed to a struct
// or any other type.
func isMapBackedFactory(factory func() any) bool {
	if factory == nil {
		return false
	}
	t := reflect.TypeOf(factory())
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t != nil && t == mapStringAnyType
}

var mapStringAnyType = reflect.TypeOf(map[string]any{})

// newMapBackedEngine builds a mixed-mode columnar engine for a map[string]any
// collection from its explicitly declared columns. With no declared column the
// engine cannot infer its columns, so every operation reports a descriptive
// error (REQ:mixed-mode-selection).
func newMapBackedEngine(collection string, factory func() any, refBreak bool, cfg columnarConfig) *columnarEngine {
	if len(cfg.declared) == 0 {
		return &columnarEngine{
			collection: collection,
			factory:    factory,
			initErr:    fmt.Errorf("columnar storage for map-backed collection %q requires at least one declared column (WithDeclaredColumn)", collection),
		}
	}
	eng := &columnarEngine{
		collection: collection,
		factory:    factory,
		byName:     make(map[string]*column),
		idToSlot:   make(map[string]int),
		refBreak:   refBreak,
		stgyByCol:  cfg.strategies,
		mapBacked:  true,
	}
	eng.buildDeclaredColumns(cfg.declared)
	return eng
}

// structTypeOf returns the struct type T given a factory returning *T (or T).
// It errors when the factory is nil or the resolved type is not a struct.
func structTypeOf(factory func() any) (reflect.Type, error) {
	if factory == nil {
		return nil, fmt.Errorf("collection has no registered record type")
	}
	t := reflect.TypeOf(factory())
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("registered record type is not a struct")
	}
	return t, nil
}

// buildColumns derives one column per JSON-serializable field of the record
// type, typed to the field's Go type (or []any for interface fields), and wires
// each column's strategy (explicit when supplied, else the default typed-slice
// strategy).
func (e *columnarEngine) buildColumns() {
	for i := 0; i < e.recordType.NumField(); i++ {
		field := e.recordType.Field(i)
		name, ok := jsonFieldName(field)
		if !ok {
			continue
		}
		elemType := field.Type
		if elemType.Kind() == reflect.Interface {
			elemType = anyType
		}
		col := &column{
			name:       name,
			goField:    field.Name,
			values:     reflect.MakeSlice(reflect.SliceOf(elemType), 0, 0),
			elemType:   elemType,
			refBearing: isRefBearing(field.Type),
			engine:     e,
		}
		if stgy, ok := e.stgyByCol[name]; ok && stgy != nil {
			col.strategy = stgy
		} else {
			def := newTypedSliceStrategy(col)
			col.strategy = def
			col.defaultStgy = def
		}
		e.columns = append(e.columns, col)
		e.byName[name] = col
	}
}

// buildDeclaredColumns derives one typed column per declared column for a
// map-backed collection, in declaration order. A column's element type is the
// declared Go type (or []any when an interface type is declared), and its
// strategy is the explicit one when supplied, else the default typed-slice
// strategy. When the same name is declared more than once, the last declaration
// wins (its element type replaces the earlier one).
func (e *columnarEngine) buildDeclaredColumns(declared []declaredColumn) {
	for _, dc := range declared {
		elemType := dc.elemType
		if elemType.Kind() == reflect.Interface {
			elemType = anyType
		}
		col := &column{
			name:       dc.name,
			values:     reflect.MakeSlice(reflect.SliceOf(elemType), 0, 0),
			elemType:   elemType,
			refBearing: isRefBearing(dc.elemType),
			engine:     e,
		}
		if stgy, ok := e.stgyByCol[dc.name]; ok && stgy != nil {
			col.strategy = stgy
		} else {
			def := newTypedSliceStrategy(col)
			col.strategy = def
			col.defaultStgy = def
		}
		if existing, ok := e.byName[dc.name]; ok {
			*existing = *col // last-declaration-wins: replace in place, keep order
			continue
		}
		e.columns = append(e.columns, col)
		e.byName[dc.name] = col
	}
}

var anyType = reflect.TypeOf((*any)(nil)).Elem()

// jsonFieldName reports the JSON object key a struct field marshals to (so the
// columnar field view keys match the Serialized engine's json.Unmarshal keys),
// and whether the field is serialized at all (exported, not json:"-").
func jsonFieldName(field reflect.StructField) (string, bool) {
	if field.PkgPath != "" { // unexported
		return "", false
	}
	tag, ok := field.Tag.Lookup("json")
	if !ok {
		return field.Name, true
	}
	name, _, _ := strings.Cut(tag, ",")
	switch name {
	case "-":
		return "", false
	case "":
		return field.Name, true
	default:
		return name, true
	}
}

// isRefBearing reports whether a field type can share references with the
// caller's value (slice, map, pointer, interface, or a struct/array containing
// one). Such columns are deep-copied via a JSON round-trip on write when the
// collection is faithful.
func isRefBearing(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return true
	case reflect.Array:
		return isRefBearing(t.Elem())
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).PkgPath == "" && isRefBearing(t.Field(i).Type) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (e *columnarEngine) exists(id string) bool {
	if e.initErr != nil {
		return false
	}
	slot, ok := e.idToSlot[id]
	return ok && e.live[slot]
}

func (e *columnarEngine) store(id string, record dal.Record, overwrite bool) error {
	if e.initErr != nil {
		return e.initErr
	}
	if !overwrite {
		if slot, ok := e.idToSlot[id]; ok && e.live[slot] {
			return fmt.Errorf("record already exists: %s", record.Key())
		}
	}
	values, leftover, err := e.prepareWrite(record.Data())
	if err != nil {
		return err
	}
	slot := e.allocSlot(id)
	e.writeSlot(slot, values, leftover)
	return nil
}

func (e *columnarEngine) load(id string, record dal.Record) error {
	if e.initErr != nil {
		return e.initErr
	}
	slot, ok := e.idToSlot[id]
	if !ok || !e.live[slot] {
		return dal.NewErrNotFoundByKey(record.Key(), nil)
	}
	return e.materializeSlot(slot, record.Data())
}

func (e *columnarEngine) delete(id string) {
	if e.initErr != nil {
		return
	}
	slot, ok := e.idToSlot[id]
	if !ok || !e.live[slot] {
		return
	}
	e.freeSlot(slot)
	delete(e.idToSlot, id)
	e.maybeCompact()
}

func (e *columnarEngine) update(id string, updates map[string]any) error {
	if e.initErr != nil {
		return e.initErr
	}
	slot, ok := e.idToSlot[id]
	if !ok || !e.live[slot] {
		return dal.NewErrNotFoundByKey(dal.NewKeyWithID(e.collection, id), nil)
	}
	current, err := e.rowData(slot)
	if err != nil {
		return err
	}
	for fieldName, value := range updates {
		// On the struct path an unknown field is rejected; on the map-backed
		// path an undeclared field is a valid leftover field.
		if _, ok := e.byName[fieldName]; !ok && !e.mapBacked {
			return fmt.Errorf("record for collection %q does not conform to the schema: unknown field %q", e.collection, fieldName)
		}
		current[fieldName] = value
	}
	values, leftover, err := e.prepareWrite(current)
	if err != nil {
		return err
	}
	e.writeSlot(slot, values, leftover)
	return nil
}

func (e *columnarEngine) rows() ([]engineRow, error) {
	if e.initErr != nil {
		return nil, e.initErr
	}
	ids := make([]string, 0, len(e.idToSlot))
	for id := range e.idToSlot {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return e.buildRows(ids)
}

// buildRows reassembles the given (sorted) ids into engineRows, each carrying
// the JSON-normalized field view and a materialize callback bound to the row's
// current slot.
func (e *columnarEngine) buildRows(ids []string) ([]engineRow, error) {
	rows := make([]engineRow, 0, len(ids))
	for _, id := range ids {
		slot := e.idToSlot[id]
		data, err := e.rowData(slot)
		if err != nil {
			return nil, err
		}
		s := slot
		rows = append(rows, engineRow{
			id:          id,
			data:        data,
			materialize: func(target any) error { return e.materializeSlot(s, target) },
		})
	}
	return rows, nil
}

// candidateRows implements the equality-WHERE acceleration the query path uses:
// for the single supported field==value predicate it consults the target
// column's strategy. When the strategy returns a slot set (ok=true), only those
// live slots are reassembled; "no opinion" returns ok=false and the caller
// scans all rows. The caller re-applies matchesWhere to the returned rows, so
// the result is identical to a full scan regardless of the strategy.
func (e *columnarEngine) candidateRows(condition dal.Condition) ([]engineRow, bool, error) {
	if e.initErr != nil {
		return nil, false, e.initErr
	}
	slots, ok := e.candidateSlots(condition)
	if !ok {
		return nil, false, nil
	}
	ids := make([]string, 0, len(slots))
	for slot := range slots {
		if slot >= 0 && slot < len(e.live) && e.live[slot] {
			ids = append(ids, e.slotToID[slot])
		}
	}
	sort.Strings(ids)
	rows, err := e.buildRows(ids)
	if err != nil {
		return nil, false, err
	}
	return rows, true, nil
}

// candidateSlots returns the live slots a single equality predicate selects via
// the target column's strategy, with ok=true; ok=false means "no opinion"
// (caller scans). It is only consulted for the single field==value predicate
// the adapter supports; any other condition shape yields no opinion.
func (e *columnarEngine) candidateSlots(condition dal.Condition) (SlotSet, bool) {
	comparison, ok := condition.(dal.Comparison)
	if !ok || comparison.Operator != dal.Equal {
		return nil, false
	}
	field, ok := comparison.Left.(dal.FieldRef)
	if !ok {
		return nil, false
	}
	constant, ok := comparison.Right.(dal.Constant)
	if !ok {
		return nil, false
	}
	col, ok := e.byName[field.Name()]
	if !ok {
		return nil, false
	}
	return col.strategy.EqualSlots(constant.Value)
}

// allocSlot reserves a slot for id: it reuses a free (tombstoned) slot when one
// is available, otherwise appends a new slot to every column. The id's slot is
// recorded and stays stable until deletion or compaction.
func (e *columnarEngine) allocSlot(id string) int {
	if slot, ok := e.idToSlot[id]; ok && e.live[slot] {
		return slot
	}
	if n := len(e.freeList); n > 0 {
		slot := e.freeList[n-1]
		e.freeList = e.freeList[:n-1]
		e.deadCount--
		e.live[slot] = true
		e.slotToID[slot] = id
		e.idToSlot[id] = slot
		return slot
	}
	slot := len(e.live)
	for _, col := range e.columns {
		col.values = reflect.Append(col.values, reflect.Zero(col.elemType))
	}
	if e.mapBacked {
		e.leftover = append(e.leftover, nil)
	}
	e.live = append(e.live, true)
	e.slotToID = append(e.slotToID, id)
	e.idToSlot[id] = slot
	return slot
}

// writeSlot stores already-decoded per-column values at slot, updates each
// column's strategy write side, and (on the map-backed path) stores the row's
// leftover map of undeclared fields at the same slot.
func (e *columnarEngine) writeSlot(slot int, values []reflect.Value, leftover map[string]any) {
	for i, col := range e.columns {
		col.values.Index(slot).Set(values[i])
		col.strategy.SetValue(slot, values[i].Interface())
	}
	if e.mapBacked {
		e.leftover[slot] = leftover
	}
}

// freeSlot tombstones a slot: it marks it dead, zeroes its column cells, clears
// each strategy, drops its leftover map (map-backed), and pushes it to the free
// list for reuse.
func (e *columnarEngine) freeSlot(slot int) {
	for _, col := range e.columns {
		col.values.Index(slot).Set(reflect.Zero(col.elemType))
		col.strategy.ClearValue(slot)
	}
	if e.mapBacked {
		e.leftover[slot] = nil
	}
	e.live[slot] = false
	e.slotToID[slot] = ""
	e.freeList = append(e.freeList, slot)
	e.deadCount++
}

// prepareWrite decodes a record's data into per-column values and, on the
// map-backed path, the leftover map of undeclared fields. On the struct path the
// leftover return is nil. Decoding happens before any slot is reserved, so a
// decode error (non-serializable value, unknown field on the struct path, or a
// declared value that cannot be stored as its column type) leaves nothing stored
// for the record.
func (e *columnarEngine) prepareWrite(data any) ([]reflect.Value, map[string]any, error) {
	if e.mapBacked {
		return e.decodeMapFields(data)
	}
	values, err := e.decodeFields(data)
	return values, nil, err
}

// decodeMapFields decodes a map-backed record: each declared column's value is
// decoded into its typed cell (a clearly-incompatible value is a descriptive
// error) and removed from the leftover, while every undeclared field is
// deep-copied (JSON round-trip, faithful default) into the leftover map at the
// row's slot. A record with only declared fields yields a nil leftover.
func (e *columnarEngine) decodeMapFields(data any) ([]reflect.Value, map[string]any, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, nil, err
	}
	var asMap map[string]any
	if err := json.Unmarshal(b, &asMap); err != nil {
		return nil, nil, err
	}
	values := make([]reflect.Value, len(e.columns))
	for i, col := range e.columns {
		dst := reflect.New(col.elemType)
		if raw, present := asMap[col.name]; present {
			if err := assignInto(dst, raw, col); err != nil {
				return nil, nil, fmt.Errorf("declared column %q of collection %q: %w", col.name, e.collection, err)
			}
		}
		values[i] = dst.Elem()
	}
	var leftover map[string]any
	for key, raw := range asMap {
		if _, declared := e.byName[key]; declared {
			continue // declared keys live in typed columns, not in leftover
		}
		if leftover == nil {
			leftover = make(map[string]any)
		}
		leftover[key] = raw // raw is the JSON-decoded value: already ref-isolated
	}
	return values, leftover, nil
}

// decodeFields converts a record's data (a struct, pointer, or map[string]any)
// into one reflect.Value per column, typed to the column's element type.
//
// The record is first marshaled (rejecting non-serializable values) and
// unmarshaled into a normalized map, matching the Serialized engine's
// validation and reassembly view; an unknown field is rejected there. Scalar
// columns are then decoded from the normalized value. A reference-bearing
// column is deep-copied via the round-trip when the collection is faithful;
// when the opt-out is set and the source is a live struct, the caller's field
// value is stored by direct reflection copy (sharing references). Fields absent
// from the data decode to the column's zero value.
func (e *columnarEngine) decodeFields(data any) ([]reflect.Value, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var asMap map[string]any
	if err := json.Unmarshal(b, &asMap); err != nil {
		return nil, err
	}
	for key := range asMap {
		if _, ok := e.byName[key]; !ok {
			return nil, fmt.Errorf("record for collection %q does not conform to the schema: unknown field %q", e.collection, key)
		}
	}
	src := structValue(data)
	values := make([]reflect.Value, len(e.columns))
	for i, col := range e.columns {
		dst := reflect.New(col.elemType)
		if !e.refBreak && col.refBearing && src.IsValid() {
			if field := src.FieldByName(col.goField); field.IsValid() && field.Type() == col.elemType {
				dst.Elem().Set(field)
				values[i] = dst.Elem()
				continue
			}
		}
		raw, present := asMap[col.name]
		if present {
			if err := assignInto(dst, raw, col); err != nil {
				return nil, err
			}
		}
		values[i] = dst.Elem()
	}
	return values, nil
}

// structValue returns the underlying struct reflect.Value of a record's data
// (dereferencing a pointer), or an invalid Value when the data is not a struct
// (e.g. a map[string]any write), in which case the opt-out direct copy does not
// apply and the round-trip path is used.
func structValue(data any) reflect.Value {
	v := reflect.ValueOf(data)
	for v.IsValid() && v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.IsValid() && v.Kind() == reflect.Struct {
		return v
	}
	return reflect.Value{}
}

// assignInto decodes a JSON-normalized value into a freshly allocated cell of
// the column's element type. The []any fallback keeps the raw normalized value;
// any other type is decoded via json.Unmarshal into the typed cell.
func assignInto(dst reflect.Value, raw any, col *column) error {
	if col.elemType == anyType {
		if raw != nil {
			dst.Elem().Set(reflect.ValueOf(raw))
		}
		return nil
	}
	// raw is a value decoded from JSON (see decodeFields), so it always
	// re-marshals; only the decode into the typed cell can fail (type mismatch).
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, dst.Interface()); err != nil {
		return err
	}
	return nil
}

// rowData builds the JSON-normalized map[string]any field view for a slot by
// marshaling each column cell, matching the Serialized engine's view exactly
// (numbers as float64, nested as map[string]any), so the shared query pipeline
// produces identical results.
func (e *columnarEngine) rowData(slot int) (map[string]any, error) {
	cells := make(map[string]any, len(e.columns)+len(e.leftoverAt(slot)))
	for key, value := range e.leftoverAt(slot) {
		cells[key] = value
	}
	for _, col := range e.columns {
		cells[col.name] = col.values.Index(slot).Interface()
	}
	b, err := json.Marshal(cells)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	// b is the marshaling of a map, so it is a JSON object that always decodes
	// back into map[string]any.
	_ = json.Unmarshal(b, &out)
	return out, nil
}

// materializeSlot reassembles a slot into a typed target by marshaling the
// per-column cells into a JSON object and unmarshaling it into target, so the
// result shares no references with stored data or other reads.
func (e *columnarEngine) materializeSlot(slot int, target any) error {
	obj := make(map[string]any, len(e.columns)+len(e.leftoverAt(slot)))
	for key, value := range e.leftoverAt(slot) {
		obj[key] = value
	}
	for _, col := range e.columns {
		obj[col.name] = col.values.Index(slot).Interface()
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

// leftoverAt returns the slot's leftover map of undeclared fields on the
// map-backed path, or nil on the struct path (and for an empty leftover). Ranging
// over a nil map is a no-op, so callers merge it unconditionally.
func (e *columnarEngine) leftoverAt(slot int) map[string]any {
	if !e.mapBacked {
		return nil
	}
	return e.leftover[slot]
}

// maybeCompact compacts when the dead-slot fraction crosses the threshold,
// reclaiming dead slots while preserving every live record. It runs under the
// global write lock; readers never observe partial state because no read can
// interleave with it.
func (e *columnarEngine) maybeCompact() {
	total := len(e.live)
	if total == 0 || e.deadCount == 0 {
		return
	}
	if float64(e.deadCount)/float64(total) < compactionThreshold {
		return
	}
	e.compact()
}

// compact rebuilds the column slices and slot mapping with only live records,
// in ascending current-slot order, then rebuilds each column's strategy from
// the compacted slices.
func (e *columnarEngine) compact() {
	liveSlots := make([]int, 0, len(e.live)-e.deadCount)
	for slot, isLive := range e.live {
		if isLive {
			liveSlots = append(liveSlots, slot)
		}
	}
	newLen := len(liveSlots)
	for _, col := range e.columns {
		next := reflect.MakeSlice(col.values.Type(), newLen, newLen)
		for newSlot, oldSlot := range liveSlots {
			next.Index(newSlot).Set(col.values.Index(oldSlot))
		}
		col.values = next
	}
	var newLeftover []map[string]any
	if e.mapBacked {
		newLeftover = make([]map[string]any, newLen)
		for newSlot, oldSlot := range liveSlots {
			newLeftover[newSlot] = e.leftover[oldSlot]
		}
	}
	newSlotToID := make([]string, newLen)
	newLive := make([]bool, newLen)
	for newSlot, oldSlot := range liveSlots {
		id := e.slotToID[oldSlot]
		newSlotToID[newSlot] = id
		newLive[newSlot] = true
		e.idToSlot[id] = newSlot
	}
	e.leftover = newLeftover
	e.slotToID = newSlotToID
	e.live = newLive
	e.freeList = e.freeList[:0]
	e.deadCount = 0
	e.rebuildStrategies()
}

// rebuildStrategies re-syncs each column's strategy with the compacted slices:
// the default strategy reads the live slices directly (nothing to do), while an
// explicit external strategy is notified via its write side per live slot.
func (e *columnarEngine) rebuildStrategies() {
	for _, col := range e.columns {
		if col.defaultStgy != nil {
			continue
		}
		for slot := 0; slot < col.values.Len(); slot++ {
			col.strategy.SetValue(slot, col.values.Index(slot).Interface())
		}
	}
}
