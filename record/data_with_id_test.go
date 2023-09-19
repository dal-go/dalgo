package record

import (
	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewDataWithID(t *testing.T) {
	type data struct {
		Title string
	}

	type args[K comparable] struct {
		id   K
		key  *dal.Key
		data *data
	}

	type testCase[K comparable] struct {
		name string
		args args[K]
		want DataWithID[K, *data]
	}
	d1 := data{Title: "test"}
	tests := []testCase[string]{
		{
			name: "should_pass",
			args: args[string]{
				id:   "r1",
				key:  dal.NewKeyWithID("r1", "SomeCollection"),
				data: &data{Title: "test"},
			},
			want: DataWithID[string, *data]{
				WithID: WithID[string]{
					ID:     "r1",
					Key:    dal.NewKeyWithID("r1", "SomeCollection"),
					Record: dal.NewRecordWithData(dal.NewKeyWithID("r1", "SomeCollection"), &d1),
				},
				Data: &d1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewDataWithID(tt.args.id, tt.args.key, tt.args.data)
			assert.Equal(t, tt.want.ID, got.ID)
			assert.Equal(t, tt.want.Key, got.Key)
			assert.Equal(t, tt.want.Data, got.Data)
			got.Record.SetError(nil)
			assert.Equal(t, tt.want.Data, got.Record.Data())
		})
	}
	t.Run("should_panic_on_pointer_to_a_pointer", func(t *testing.T) {
		assert.Panics(t, func() {
			d1 := data{Title: "test"}
			d2 := &d1
			d3 := &d2
			NewDataWithID("r1", dal.NewKeyWithID("r1", "SomeCollection"), d3)
		})
	})
	t.Run("should_panic_on_pointer_to_an_interface", func(t *testing.T) {
		assert.Panics(t, func() {
			d1 := data{Title: "test"}
			var d2 any = &d1
			var d3 = &d2
			NewDataWithID("r1", dal.NewKeyWithID("r1", "SomeCollection"), d3)
		})
	})
}
