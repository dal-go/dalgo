package recordset

import (
	"testing"
)

func TestFormat(t *testing.T) {
	formatFunc := func(value any) string {
		return "formatted"
	}

	option := Format(formatFunc)
	options := &ColumnOptions{}
	option(options)

	if options.format == nil {
		t.Fatal("options.format is nil")
	}

	if options.format(nil) != "formatted" {
		t.Errorf("expected 'formatted', got '%s'", options.format(nil))
	}
}

func TestColDbType(t *testing.T) {
	option := ColDbType("STRING")
	options := &ColumnOptions{}
	option(options)
	if options.dbType != "STRING" {
		t.Errorf("expected 'STRING', got '%s'", options.dbType)
	}
}
