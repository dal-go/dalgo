package record

import (
	"fmt"
	"testing"
)

func TestWithID_String(t *testing.T) {
	tests := []struct {
		name  string
		input fmt.Stringer
		want  string
	}{
		{
			name:  "Empty FullID",
			input: WithID[string]{ID: "1", FullID: "", Key: nil, Record: nil},
			want:  "{ID=1, FullID=nil, Key=<nil>, Record=<nil>}",
		},
		{
			name:  "FullID is not empty, ID is string",
			input: WithID[string]{ID: "1", FullID: "custom-1", Key: nil, Record: nil},
			want:  `{ID="1", FullID="custom-1", Key=<nil>, Record=<nil>}`,
		},
		{
			name:  "FullID is not empty, ID is integer",
			input: WithID[int]{ID: 1, FullID: "custom-1", Key: nil, Record: nil},
			want:  `{ID=1, FullID="custom-1", Key=<nil>, Record=<nil>}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.String()
			if got != tt.want {
				t.Errorf("WithID.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
