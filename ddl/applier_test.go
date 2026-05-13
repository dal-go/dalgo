package ddl

import (
	"context"
	"errors"
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

func TestApplyTo_AddField(t *testing.T) {
	t.Parallel()
	rec := &recordingApplier{}
	ctx := context.Background()
	f := dbschema.FieldDef{Name: dal.FieldName("email"), Type: dbschema.String, Nullable: false}
	op := AddField(f, IfNotExists())

	if err := op.ApplyTo(ctx, rec); err != nil {
		t.Fatalf("ApplyTo: unexpected error: %v", err)
	}
	if rec.addFieldCalls != 1 {
		t.Errorf("addFieldCalls = %d, want 1", rec.addFieldCalls)
	}
	if rec.addField.Name != f.Name || rec.addField.Type != f.Type {
		t.Errorf("addField = %+v, want %+v", rec.addField, f)
	}
	if !rec.lastOpts.IfNotExists {
		t.Error("IfNotExists option not forwarded")
	}
}

func TestApplyTo_DropField(t *testing.T) {
	t.Parallel()
	rec := &recordingApplier{}
	ctx := context.Background()
	op := DropField(dal.FieldName("email"), IfExists())

	if err := op.ApplyTo(ctx, rec); err != nil {
		t.Fatalf("ApplyTo: unexpected error: %v", err)
	}
	if rec.dropFieldCalls != 1 {
		t.Errorf("dropFieldCalls = %d, want 1", rec.dropFieldCalls)
	}
	if string(rec.dropField) != "email" {
		t.Errorf("dropField = %q, want email", rec.dropField)
	}
	if !rec.lastOpts.IfExists {
		t.Error("IfExists option not forwarded")
	}
}

func TestApplyTo_ModifyField(t *testing.T) {
	t.Parallel()
	rec := &recordingApplier{}
	ctx := context.Background()
	newDef := dbschema.FieldDef{Name: dal.FieldName("email"), Type: dbschema.String, Nullable: false}
	op := ModifyField(dal.FieldName("email"), newDef)

	if err := op.ApplyTo(ctx, rec); err != nil {
		t.Fatalf("ApplyTo: unexpected error: %v", err)
	}
	if rec.modifyFieldCalls != 1 {
		t.Errorf("modifyFieldCalls = %d, want 1", rec.modifyFieldCalls)
	}
	if string(rec.modifyOld) != "email" {
		t.Errorf("modifyOld = %q, want email", rec.modifyOld)
	}
	if rec.modifyNew.Type != dbschema.String {
		t.Errorf("modifyNew.Type = %v, want String", rec.modifyNew.Type)
	}
}

func TestApplyTo_RenameField(t *testing.T) {
	t.Parallel()
	rec := &recordingApplier{}
	ctx := context.Background()
	op := RenameField(dal.FieldName("email"), dal.FieldName("email_address"))

	if err := op.ApplyTo(ctx, rec); err != nil {
		t.Fatalf("ApplyTo: unexpected error: %v", err)
	}
	if rec.renameFieldCalls != 1 {
		t.Errorf("renameFieldCalls = %d, want 1", rec.renameFieldCalls)
	}
	if string(rec.renameOld) != "email" || string(rec.renameNew) != "email_address" {
		t.Errorf("rename = (%q, %q), want (email, email_address)", rec.renameOld, rec.renameNew)
	}
}

func TestApplyTo_AddIndex(t *testing.T) {
	t.Parallel()
	rec := &recordingApplier{}
	ctx := context.Background()
	idx := dbschema.IndexDef{Name: "ix_users_email", Collection: "users", Fields: []dal.FieldName{"email"}}
	op := AddIndex(idx)

	if err := op.ApplyTo(ctx, rec); err != nil {
		t.Fatalf("ApplyTo: unexpected error: %v", err)
	}
	if rec.addIndexCalls != 1 {
		t.Errorf("addIndexCalls = %d, want 1", rec.addIndexCalls)
	}
	if rec.addIndex.Name != idx.Name {
		t.Errorf("addIndex.Name = %q, want %q", rec.addIndex.Name, idx.Name)
	}
}

func TestApplyTo_DropIndex(t *testing.T) {
	t.Parallel()
	rec := &recordingApplier{}
	ctx := context.Background()
	op := DropIndex("ix_users_email", IfExists())

	if err := op.ApplyTo(ctx, rec); err != nil {
		t.Fatalf("ApplyTo: unexpected error: %v", err)
	}
	if rec.dropIndexCalls != 1 {
		t.Errorf("dropIndexCalls = %d, want 1", rec.dropIndexCalls)
	}
	if rec.dropIndex != "ix_users_email" {
		t.Errorf("dropIndex = %q, want ix_users_email", rec.dropIndex)
	}
	if !rec.lastOpts.IfExists {
		t.Error("IfExists option not forwarded")
	}
}

// TestApplyTo_PropagatesError verifies that ApplyTo returns the
// Applier's error verbatim (errors.Is identity).
func TestApplyTo_PropagatesError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("driver-side failure")
	rec := &recordingApplier{returnErr: sentinel}
	ctx := context.Background()
	op := AddField(dbschema.FieldDef{Name: dal.FieldName("x"), Type: dbschema.Int})
	got := op.ApplyTo(ctx, rec)
	if !errors.Is(got, sentinel) {
		t.Errorf("ApplyTo error = %v, want %v (via errors.Is)", got, sentinel)
	}
}

// TestAlterOp_InterfaceHasApplyTo verifies the AlterOp interface
// itself now declares ApplyTo (catches accidental removal).
func TestAlterOp_InterfaceHasApplyTo(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf((*AlterOp)(nil)).Elem()
	var found bool
	for i := 0; i < typ.NumMethod(); i++ {
		if typ.Method(i).Name == "ApplyTo" {
			found = true
			break
		}
	}
	if !found {
		t.Error("AlterOp interface missing ApplyTo method")
	}
}
