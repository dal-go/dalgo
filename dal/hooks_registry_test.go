package dal

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

// The hook registries are process-wide, so these tests are white-box: they save
// and restore the slices directly. They must not run in parallel.
func withHooks(t *testing.T, save []RecordHook, del []RecordHook) {
	t.Helper()
	prevSave, prevDelete := beforeSaveHooks, beforeDeleteHooks
	beforeSaveHooks, beforeDeleteHooks = save, del
	t.Cleanup(func() {
		beforeSaveHooks, beforeDeleteHooks = prevSave, prevDelete
	})
}

// hookSpySession records the writes that get past the hooks.
type hookSpySession struct {
	writes []string
}

func (s *hookSpySession) Insert(context.Context, record.Record, ...InsertOption) error {
	s.writes = append(s.writes, "Insert")
	return nil
}
func (s *hookSpySession) InsertMulti(context.Context, []record.Record, ...InsertOption) error {
	s.writes = append(s.writes, "InsertMulti")
	return nil
}
func (s *hookSpySession) Set(context.Context, record.Record) error {
	s.writes = append(s.writes, "Set")
	return nil
}
func (s *hookSpySession) SetMulti(context.Context, []record.Record) error {
	s.writes = append(s.writes, "SetMulti")
	return nil
}
func (s *hookSpySession) Update(context.Context, *record.Key, []update.Update, ...Precondition) error {
	s.writes = append(s.writes, "Update")
	return nil
}
func (s *hookSpySession) UpdateRecord(context.Context, record.Record, []update.Update, ...Precondition) error {
	s.writes = append(s.writes, "UpdateRecord")
	return nil
}
func (s *hookSpySession) UpdateMulti(context.Context, []*record.Key, []update.Update, ...Precondition) error {
	s.writes = append(s.writes, "UpdateMulti")
	return nil
}
func (s *hookSpySession) Delete(context.Context, *record.Key) error {
	s.writes = append(s.writes, "Delete")
	return nil
}
func (s *hookSpySession) DeleteMulti(context.Context, []*record.Key) error {
	s.writes = append(s.writes, "DeleteMulti")
	return nil
}

var _ WriteSession = (*hookSpySession)(nil)

func hookPipeline(s WriteSession) writePipeline {
	return writePipeline{ws: s, validate: true}
}

type hookData struct{ Name string }

func hookRecord() record.Record {
	return record.NewRecordWithData(record.NewKeyWithID("things", "t1"), &hookData{Name: "t1"})
}

// TestBeforeSaveHookRunsAndCanAbortTheWrite: registered before-save hooks were
// unreachable dead code before this change (nothing called BeforeSave, and
// there was no way to register one). If it goes red, hooks silently stop
// running — the failure mode this whole work exists to remove.
func TestBeforeSaveHookRunsAndCanAbortTheWrite(t *testing.T) {
	boom := errors.New("hook says no")
	var seen []*record.Key
	withHooks(t, []RecordHook{
		func(_ context.Context, r record.Record) error {
			seen = append(seen, r.Key())
			return boom
		},
	}, nil)

	spy := &hookSpySession{}
	err := hookPipeline(spy).Insert(context.Background(), hookRecord())

	if !errors.Is(err, ErrHookFailed) {
		t.Fatalf("err = %v, want ErrHookFailed", err)
	}
	if len(seen) != 1 {
		t.Fatalf("hook ran %d times, want 1", len(seen))
	}
	if len(spy.writes) != 0 {
		t.Fatalf("a hook rejected the write but it still reached the backend: %v", spy.writes)
	}
}

// TestBeforeSaveHookStillRunsWithoutValidation: WithoutValidation opts out of
// validation, not out of the pipeline. If it goes red, a repair migration would
// also silently skip audit/stamping hooks.
func TestBeforeSaveHookStillRunsWithoutValidation(t *testing.T) {
	var ran int
	withHooks(t, []RecordHook{
		func(context.Context, record.Record) error {
			ran++
			return nil
		},
	}, nil)

	spy := &hookSpySession{}
	p := WithoutValidation(hookPipeline(spy))
	if err := p.Insert(context.Background(), hookRecord()); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if ran != 1 {
		t.Fatalf("before-save hook ran %d times, want 1", ran)
	}
}

// TestBeforeDeleteHookRunsAndCanAbortTheDelete implements the decision that
// Delete participates in the pipeline: no data to validate, but hooks run.
func TestBeforeDeleteHookRunsAndCanAbortTheDelete(t *testing.T) {
	boom := errors.New("hook says no")
	var seen []*record.Key
	withHooks(t, nil, []RecordHook{
		func(_ context.Context, r record.Record) error {
			seen = append(seen, r.Key())
			return boom
		},
	})

	spy := &hookSpySession{}
	key := record.NewKeyWithID("things", "t1")
	err := hookPipeline(spy).Delete(context.Background(), key)

	if !errors.Is(err, ErrHookFailed) {
		t.Fatalf("err = %v, want ErrHookFailed", err)
	}
	if len(seen) != 1 || seen[0].String() != key.String() {
		t.Fatalf("before-delete hook saw %v, want exactly the deleted key", seen)
	}
	if len(spy.writes) != 0 {
		t.Fatalf("a hook rejected the delete but it still reached the backend: %v", spy.writes)
	}
}

// TestDeleteMultiRejectsWholeBatchWhenOneKeyIsRejected mirrors the record-bearing
// batch rule for deletes.
func TestDeleteMultiRejectsWholeBatchWhenOneKeyIsRejected(t *testing.T) {
	keys := []*record.Key{
		record.NewKeyWithID("things", "t1"),
		record.NewKeyWithID("things", "t2"),
		record.NewKeyWithID("things", "t3"),
	}
	withHooks(t, nil, []RecordHook{
		func(_ context.Context, r record.Record) error {
			if r.Key().ID == "t3" {
				return errors.New("hook says no")
			}
			return nil
		},
	})

	spy := &hookSpySession{}
	if err := hookPipeline(spy).DeleteMulti(context.Background(), keys); !errors.Is(err, ErrHookFailed) {
		t.Fatalf("err = %v, want ErrHookFailed", err)
	}
	if len(spy.writes) != 0 {
		t.Fatalf("part of a rejected delete batch reached the backend: %v", spy.writes)
	}
}

// TestAddBeforeSaveHookRegisters proves the registration API actually feeds the
// registry the pipeline reads. Without it the hook slice is unreachable, which
// is how it stayed dead (and misspelled) for so long.
func TestAddBeforeSaveHookRegisters(t *testing.T) {
	withHooks(t, nil, nil)
	var ran bool
	AddBeforeSaveHook(func(context.Context, record.Record) error {
		ran = true
		return nil
	})
	spy := &hookSpySession{}
	if err := hookPipeline(spy).Set(context.Background(), hookRecord()); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !ran {
		t.Fatal("a hook registered with AddBeforeSaveHook did not run")
	}
}

// TestAddBeforeDeleteHookRegisters is the delete-side twin.
func TestAddBeforeDeleteHookRegisters(t *testing.T) {
	withHooks(t, nil, nil)
	var ran bool
	AddBeforeDeleteHook(func(context.Context, record.Record) error {
		ran = true
		return nil
	})
	spy := &hookSpySession{}
	if err := hookPipeline(spy).Delete(context.Background(), record.NewKeyWithID("things", "t1")); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !ran {
		t.Fatal("a hook registered with AddBeforeDeleteHook did not run")
	}
}

// TestValidationRunsOnAFreshRecord is the regression test for the reason this
// code path could never have worked: record.Record.Data() panics until the
// record's retrieval state is set, and a record about to be inserted has never
// been read. If it goes red, every first write of a validatable record panics.
func TestValidationRunsOnAFreshRecord(t *testing.T) {
	fresh := record.NewRecordWithData(record.NewKeyWithID("things", "t1"), &hookData{Name: "t1"})
	got := recordDataToValidate(fresh)
	if data, ok := got.(*hookData); !ok || data.Name != "t1" {
		t.Fatalf("recordDataToValidate returned %#v, want the record's *hookData", got)
	}
}
