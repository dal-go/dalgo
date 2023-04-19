package dal

import (
	"context"
	"fmt"
)

type ValidatableRecord interface {
	Validate() error
}

var beforeSafeHooks []RecordHook

func BeforeSave(c context.Context, db Database, record Record) error {
	if err := beforeSafe(c, db, record); err != nil {
		return err
	}
	return callRecordHooks(c, record, beforeSafeHooks)
}

func callRecordHooks(c context.Context, record Record, hooks []RecordHook) error {
	//errs := make([]error, 0, len(hooks))
	for _, hook := range hooks {
		if err := hook(c, record); err != nil {
			return fmt.Errorf("%w: %v", ErrHookFailed, err)
		}
	}
	//if len(errs) > 0 {
	//	return fmt.Errorf("%w: %v", ErrHookFailed, errors.Join(errs...))
	//}
	return nil
}

func beforeSafe(c context.Context, db Database, record Record) error {
	data := record.Data()
	if validatable, ok := data.(ValidatableRecord); ok {
		if err := validatable.Validate(); err != nil {
			return err
		}
	}
	return nil
}
