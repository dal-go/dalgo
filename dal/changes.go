package dal

import (
	"context"
	"fmt"

	"github.com/dal-go/record"
)

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
