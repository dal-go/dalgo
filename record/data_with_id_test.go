package record

import (
	"github.com/dal-go/dalgo/dal"
	"reflect"
	"testing"
)

func TestNewDataWithID(t *testing.T) {
	type data struct {
		Title string
	}

	type args[K comparable, D any] struct {
		id   K
		key  *dal.Key
		data D
	}

	type testCase[K comparable, D any] struct {
		name string
		args args[K, D]
		want DataWithID[K, D]
	}
	tests := []testCase[string, data]{
		{
			name: "should_pass",
			args: args[string, data]{
				id:   "r1",
				key:  dal.NewKeyWithID("r1", "SomeCollection"),
				data: data{Title: "test"},
			},
			want: DataWithID[string, data]{
				WithID: WithID[string]{
					ID:     "r1",
					Key:    dal.NewKeyWithID("r1", "SomeCollection"),
					Record: dal.NewRecordWithData(dal.NewKeyWithID("r1", "SomeCollection"), data{Title: "test"}),
				},
				Data: data{Title: "test"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewDataWithID(tt.args.id, tt.args.key, tt.args.data); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewDataWithID() = %v, want %v", got, tt.want)
			}
		})
	}
}
