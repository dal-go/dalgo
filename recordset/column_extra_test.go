package recordset

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewTypedColumn_Options(t *testing.T) {
	col := NewTypedColumn("test", "default", ColDbType("STRING"))
	assert.Equal(t, "STRING", col.DbType())
}

func TestNewColumn_Options(t *testing.T) {
	col := NewColumn("test", "default", ColDbType("STRING"))
	assert.Equal(t, "STRING", col.DbType())
	assert.Equal(t, reflect.TypeOf(""), col.ValueType())
}

func TestNewBitmapColumn_Options(t *testing.T) {
	col := NewBitmapColumn("test", 0, func() string { return "default" }, ColDbType("STRING"))
	assert.Equal(t, "STRING", col.DbType())
	assert.Equal(t, "default", col.DefaultValue())
}
