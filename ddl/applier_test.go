package ddl

import (
	"context"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
)

// recordingApplier captures each method call for assertion. Fields
// hold the most-recent call's arguments per method; unused fields
// stay zero. The numbered counters let tests verify which method was
// called.
type recordingApplier struct {
	addFieldCalls    int
	dropFieldCalls   int
	modifyFieldCalls int
	renameFieldCalls int
	addIndexCalls    int
	dropIndexCalls   int

	lastCtx context.Context

	addField  dbschema.FieldDef
	dropField dal.FieldName
	modifyOld dal.FieldName
	modifyNew dbschema.FieldDef
	renameOld dal.FieldName
	renameNew dal.FieldName
	addIndex  dbschema.IndexDef
	dropIndex string

	lastOpts Options

	// returnErr is returned from every ApplyXxx call when non-nil.
	// Used for error-propagation tests.
	returnErr error
}

func (r *recordingApplier) ApplyAddField(ctx context.Context, f dbschema.FieldDef, opts Options) error {
	r.addFieldCalls++
	r.lastCtx = ctx
	r.addField = f
	r.lastOpts = opts
	return r.returnErr
}

func (r *recordingApplier) ApplyDropField(ctx context.Context, name dal.FieldName, opts Options) error {
	r.dropFieldCalls++
	r.lastCtx = ctx
	r.dropField = name
	r.lastOpts = opts
	return r.returnErr
}

func (r *recordingApplier) ApplyModifyField(ctx context.Context, name dal.FieldName, newDef dbschema.FieldDef, opts Options) error {
	r.modifyFieldCalls++
	r.lastCtx = ctx
	r.modifyOld = name
	r.modifyNew = newDef
	r.lastOpts = opts
	return r.returnErr
}

func (r *recordingApplier) ApplyRenameField(ctx context.Context, oldName, newName dal.FieldName, opts Options) error {
	r.renameFieldCalls++
	r.lastCtx = ctx
	r.renameOld = oldName
	r.renameNew = newName
	r.lastOpts = opts
	return r.returnErr
}

func (r *recordingApplier) ApplyAddIndex(ctx context.Context, idx dbschema.IndexDef, opts Options) error {
	r.addIndexCalls++
	r.lastCtx = ctx
	r.addIndex = idx
	r.lastOpts = opts
	return r.returnErr
}

func (r *recordingApplier) ApplyDropIndex(ctx context.Context, name string, opts Options) error {
	r.dropIndexCalls++
	r.lastCtx = ctx
	r.dropIndex = name
	r.lastOpts = opts
	return r.returnErr
}

// TestApplier_InterfaceShape verifies that the Applier interface has
// exactly six methods with the expected signatures. Catches accidental
// rename / signature drift at refactor time.
func TestApplier_InterfaceShape(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf((*Applier)(nil)).Elem()
	if got, want := typ.NumMethod(), 6; got != want {
		t.Errorf("Applier method count = %d, want %d", got, want)
	}
	wantMethods := map[string]bool{
		"ApplyAddField":    false,
		"ApplyDropField":   false,
		"ApplyModifyField": false,
		"ApplyRenameField": false,
		"ApplyAddIndex":    false,
		"ApplyDropIndex":   false,
	}
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		if _, known := wantMethods[m.Name]; !known {
			t.Errorf("unexpected method %q on Applier", m.Name)
			continue
		}
		wantMethods[m.Name] = true
		// First param of every method MUST be context.Context.
		if got := m.Type.In(0); got != reflect.TypeOf((*context.Context)(nil)).Elem() {
			t.Errorf("%s first param = %v, want context.Context", m.Name, got)
		}
		// Last return MUST be error.
		if got, want := m.Type.NumOut(), 1; got != want {
			t.Errorf("%s NumOut = %d, want %d", m.Name, got, want)
			continue
		}
		if got := m.Type.Out(0); got != reflect.TypeOf((*error)(nil)).Elem() {
			t.Errorf("%s return = %v, want error", m.Name, got)
		}
	}
	for name, seen := range wantMethods {
		if !seen {
			t.Errorf("Applier missing method %q", name)
		}
	}
}

// Compile-time assertion: *recordingApplier satisfies Applier.
var _ Applier = (*recordingApplier)(nil)
