package dbschema

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
)

func intPtr(i int) *int { return &i }

func TestFieldDef_Compiles(t *testing.T) {
	// Per REQ:field-def-struct AC-1.
	f := FieldDef{
		Name:     dal.FieldName("email"),
		Type:     String,
		Length:   intPtr(255),
		Nullable: false,
	}
	assert.Equal(t, dal.FieldName("email"), f.Name)
	assert.Equal(t, String, f.Type)
	assert.Equal(t, 255, *f.Length)
	assert.False(t, f.Nullable)
}

func TestFieldDef_ZeroValue(t *testing.T) {
	// Per REQ:field-def-struct AC-2.
	var f FieldDef
	assert.Equal(t, dal.FieldName(""), f.Name)
	assert.Equal(t, Null, f.Type)
	assert.Nil(t, f.Length)
	assert.Nil(t, f.Precision)
	assert.False(t, f.Nullable)
	assert.Nil(t, f.Default)
	assert.False(t, f.AutoIncrement)
}

func TestFieldDef_DecimalPrecision(t *testing.T) {
	// Per REQ:field-def-struct AC-3.
	f := FieldDef{
		Name:      "amount",
		Type:      Decimal,
		Precision: &Precision{Total: 18, Scale: 4},
	}
	assert.Equal(t, 18, f.Precision.Total)
	assert.Equal(t, 4, f.Precision.Scale)
}

func TestFieldDef_DefaultLiteral(t *testing.T) {
	// Per REQ:field-def-struct AC-4.
	f := FieldDef{
		Name:    "status",
		Type:    String,
		Default: DefaultLiteral{Value: "active"},
	}
	assert.NotNil(t, f.Default)
	lit, ok := f.Default.(DefaultLiteral)
	assert.True(t, ok)
	assert.Equal(t, "active", lit.Value)
}
