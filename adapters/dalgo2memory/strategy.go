package dalgo2memory

import "reflect"

// SlotSet is a set of per-row slot indices. The columnar engine assigns each
// live record a stable slot shared across all of a collection's column slices;
// a ColumnStrategy's equality read side returns the slots whose column equals a
// queried value. A set (rather than a slice) is returned so that, when the
// adapter grows multi-predicate AND WHERE, per-predicate sets can be
// intersected.
type SlotSet map[int]struct{}

// ColumnStrategy backs a single column of a columnar collection. It is exported
// so an out-of-core package (e.g. a bitmap index) can supply a strategy via
// WithColumnStrategy without dalgo2memory importing it.
//
// The engine remains the source of truth for stored values (its typed column
// slices); a strategy is an index kept in sync through the write side, used to
// accelerate the equality read side.
type ColumnStrategy interface {
	// SetValue records that the column holds value at the given slot. The engine
	// calls it on every write (insert/overwrite/update) for the column.
	SetValue(slot int, value any)

	// ClearValue records that the slot no longer holds a value for the column.
	// The engine calls it on delete and during compaction rebuilds.
	ClearValue(slot int)

	// EqualSlots returns the live slots whose column equals value, with ok=true.
	// Returning ok=false signals "no opinion": the engine falls back to scanning.
	// Equality is the adapter's value equality (Go == on comparable decoded
	// values, as in matchesWhere).
	EqualSlots(value any) (slots SlotSet, ok bool)
}

// typedSliceStrategy is the default per-column strategy. It does not keep its
// own copy of the column's values; instead it scans the engine's typed column
// slice on read, treating a slot as a candidate only when it is live. For a
// comparable element type it answers EqualSlots by scanning; for a
// non-comparable element type (e.g. an []any fallback column holding
// slices/maps) it returns "no opinion" so the engine scans.
type typedSliceStrategy struct {
	column     *column
	comparable bool
}

var _ ColumnStrategy = (*typedSliceStrategy)(nil)

// newTypedSliceStrategy builds the default strategy for a column, deciding once
// whether the column scans for equality. An []any fallback column is treated as
// non-comparable (its stored values are heterogeneous and may be
// slices/maps that cannot be compared with ==), so the strategy defers to the
// scan fall-back for it.
func newTypedSliceStrategy(col *column) *typedSliceStrategy {
	return &typedSliceStrategy{
		column:     col,
		comparable: col.elemType != anyType && col.elemType.Comparable(),
	}
}

// SetValue is a no-op for the default strategy: it reads live values directly
// from the engine's column slice, so there is nothing to record.
func (s *typedSliceStrategy) SetValue(slot int, value any) {
	_, _ = slot, value // default strategy scans the live column slice; nothing to record
}

// ClearValue is a no-op for the default strategy (see SetValue).
func (s *typedSliceStrategy) ClearValue(slot int) {
	_ = slot // default strategy scans the live column slice; nothing to clear
}

// EqualSlots scans the column slice for live slots equal to value. It returns
// "no opinion" when the element type is not comparable.
func (s *typedSliceStrategy) EqualSlots(value any) (SlotSet, bool) {
	if !s.comparable {
		return nil, false
	}
	slots := make(SlotSet)
	for slot := 0; slot < s.column.values.Len(); slot++ {
		if !s.column.engine.live[slot] {
			continue
		}
		stored := s.column.values.Index(slot).Interface()
		if valuesEqualForStrategy(stored, value) {
			slots[slot] = struct{}{}
		}
	}
	return slots, true
}

// valuesEqualForStrategy mirrors matchesWhere's equality (data[field] ==
// constant.Value): plain Go == on the decoded values, guarded so that
// comparing against a non-comparable queried value cannot panic.
func valuesEqualForStrategy(stored, queried any) bool {
	if queried != nil && !reflect.TypeOf(queried).Comparable() {
		return false
	}
	return stored == queried
}
