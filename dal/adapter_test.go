package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewAdapter(t *testing.T) {
	type args struct {
		driver  string
		version string
	}
	tests := []struct {
		name string
		args args
	}{
		{"should_pass", args{"sql", "v1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientInfo := NewAdapter(tt.args.driver, tt.args.version)
			assert.Equal(t, clientInfo.Name(), tt.args.driver)
			assert.Equal(t, clientInfo.Version(), tt.args.version)
		})
	}
}

func TestAdapter_Fields(t *testing.T) {
	type fields struct {
		driver  string
		version string
	}
	tests := []struct {
		name   string
		fields fields
		want   fields
	}{
		{"should_pass", fields{"sql", "v1"}, fields{driver: "sql", version: "v1"}},
		{"should_pass", fields{"firestore", "v2"}, fields{driver: "firestore", version: "v2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := adapter{
				name:    tt.fields.driver,
				version: tt.fields.version,
			}
			if got := v.Name(); got != tt.want.driver {
				t.Errorf("NewFieldRef() = %v, want %v", got, tt.want)
			}
			if got := v.Version(); got != tt.want.version {
				t.Errorf("Version() = %v, want %v", got, tt.want)
			}
			if s := v.String(); s != tt.want.driver+"@"+tt.want.version {
				t.Errorf("String() = %v, want %v", s, tt.want.driver+"@"+tt.want.version)
			}
		})
	}
}

func TestAdapter_Equals(t *testing.T) {
	type fields struct {
		driver  string
		version string
	}
	type args struct {
		other Adapter
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{"should_pass", fields{"sql", "v1"}, args{adapter{name: "sql", version: "v1"}}, true},
		{"should_pass", fields{"firestore", "v2"}, args{adapter{name: "firestore", version: "v2"}}, true},
		{"should_pass", fields{"firestore", "v2"}, args{adapter{name: "firestore", version: "v1"}}, false},
		{"should_pass", fields{"firestore", "v2"}, args{adapter{name: "sql", version: "v2"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := adapter{
				name:    tt.fields.driver,
				version: tt.fields.version,
			}
			if got := v.Equals(tt.args.other); got != tt.want {
				t.Errorf("Equals() = %v, want %v", got, tt.want)
			}
		})
	}
}
