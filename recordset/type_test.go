package recordset

import "testing"

func TestType_String(t *testing.T) {
	tests := []struct {
		name string
		t    Type
		want string
	}{
		{name: "UnknownType", t: UnknownType, want: "Unknown"},
		{name: "Table", t: Table, want: "Table"},
		{name: "View", t: View, want: "View"},
		{name: "StoredProcedure", t: StoredProcedure, want: "StoredProcedure"},
		{name: "Undefined", t: Type(99), want: "99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.t.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
