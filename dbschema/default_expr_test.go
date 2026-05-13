package dbschema

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultExpr_InterfaceExists(t *testing.T) {
	// Per REQ:default-expr-interface AC-1.
	var d DefaultExpr
	assert.Nil(t, d) // interface zero value is nil
}

func TestDefaultExpr_HasUnexportedMarker(t *testing.T) {
	// Per REQ:default-expr-interface AC-2 — sealed via unexported marker method.
	typ := reflect.TypeOf((*DefaultExpr)(nil)).Elem()
	assert.Equal(t, reflect.Interface, typ.Kind())
	require := typ.NumMethod()
	assert.Equal(t, 1, require, "expected exactly one method on DefaultExpr")
	method := typ.Method(0)
	// Unexported methods are reported by reflect with their package path set.
	assert.NotEmpty(t, method.PkgPath, "method %q should be unexported (have a PkgPath)", method.Name)
}

func TestDefaultLiteral_Satisfies(t *testing.T) {
	// Per REQ:default-literal AC-1.
	var d DefaultExpr = DefaultLiteral{Value: 0}
	_ = d
}

func TestDefaultLiteral_ValueAccessible(t *testing.T) {
	// Per REQ:default-literal AC-2.
	d := DefaultLiteral{Value: "guest"}
	v, ok := d.Value.(string)
	assert.True(t, ok)
	assert.Equal(t, "guest", v)
}

func TestDefaultCurrentTimestamp_Satisfies(t *testing.T) {
	// Per REQ:default-current-timestamp AC-1.
	var d DefaultExpr = DefaultCurrentTimestamp{}
	_ = d
}
