package dal

import (
	"context"
	"fmt"

	"github.com/dal-go/record"
)

// Changes tracks records that a DAL workflow intends to persist with Set or
// SetMulti. It is distinct from record.Changes, which is a declarative
// insert/update/delete command envelope executed by ApplyChanges.
type Changes struct {
	records []record.Record
}

// IsChanged reports whether a record with the same key is already tracked.
func (changes *Changes) IsChanged(rec record.Record) bool {
	if rec == nil {
		return false
	}
	for _, changed := range changes.records {
		if changed == rec || record.EqualKeys(changed.Key(), rec.Key()) {
			return true
		}
	}
	return false
}

// FlagAsChanged marks rec as changed and tracks it once per key.
func (changes *Changes) FlagAsChanged(rec record.Record) {
	if rec == nil {
		panic("record == nil")
	}
	rec.MarkAsChanged()
	if !changes.IsChanged(rec) {
		changes.records = append(changes.records, rec)
	}
}

// Records returns a copy of the tracked records.
func (changes *Changes) Records() []record.Record {
	records := make([]record.Record, len(changes.records))
	copy(records, changes.records)
	return records
}

// HasChanges reports whether at least one record is tracked.
func (changes *Changes) HasChanges() bool {
	return len(changes.records) > 0
}

// ApplyChanges applies a persistence-neutral record change set through tx.
// It clears changes only after every queued operation succeeds.
func ApplyChanges(ctx context.Context, tx ReadwriteTransaction, changes *record.Changes, excludeKeys ...*record.Key) error {
	if changes == nil {
		panic("changes == nil")
	}
	if records := excludeRecords(changes.RecordsToInsert(), excludeKeys); len(records) > 0 {
		if err := tx.InsertMulti(ctx, records); err != nil {
			return fmt.Errorf("failed to insert records: %w", err)
		}
	}
	for _, update := range changes.RecordsToUpdate {
		key := update.Record.Key()
		if err := tx.Update(ctx, key, update.Updates); err != nil {
			return fmt.Errorf("failed to update record %s: %w", key, err)
		}
	}
	if len(changes.RecordsToDelete) > 0 {
		if err := tx.DeleteMulti(ctx, changes.RecordsToDelete); err != nil {
			return fmt.Errorf("failed to delete records: %w", err)
		}
	}
	changes.Reset()
	return nil
}

func excludeRecords(records []record.Record, excludeKeys []*record.Key) []record.Record {
	if len(excludeKeys) == 0 {
		return records
	}
	result := make([]record.Record, 0, len(records))
outer:
	for _, rec := range records {
		for _, excludeKey := range excludeKeys {
			if rec.Key() == excludeKey {
				continue outer
			}
		}
		result = append(result, rec)
	}
	return result
}
