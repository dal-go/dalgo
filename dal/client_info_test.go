package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewClientInfo(t *testing.T) {
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
			clientInfo := NewClientInfo(tt.args.driver, tt.args.version)
			assert.Equal(t, clientInfo.Driver(), tt.args.driver)
			assert.Equal(t, clientInfo.Version(), tt.args.version)
		})
	}
}
