package dal

import (
	"context"
	"fmt"
)

type ValidatableRecord interface {
	Validate() error
}

var beforeSafeHooks []RecordHook

func BeforeSave(ctx context.Context, db DB, record Record) error {
	if err := beforeSafe(ctx, db, record); err != nil {
		return err
	}
	return callRecordHooks(ctx, record, beforeSafeHooks)
}

func callRecordHooks(ctx context.Context, record Record, hooks []RecordHook) error {
	//errs := make([]error, 0, len(hooks))
	for _, hook := range hooks {
		if err := hook(ctx, record); err != nil {
			return fmt.Errorf("%w: %v", ErrHookFailed, err)
		}
	}
	//if len(errs) > 0 {
	//	return fmt.Errorf("%w: %v", ErrHookFailed, errors.Join(errs...))
	//}
	return nil
}

func beforeSafe(_ context.Context, _ DB, record Record) error {
	data := record.Data()
	if validatable, ok := data.(ValidatableRecord); ok {
		if err := validatable.Validate(); err != nil {
			return err
		}
	}
	return nil
}
