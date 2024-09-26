package record

import (
	"context"
	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestWithRecordChanges_ApplyChanges(t *testing.T) {
	type fields struct {
		recordsToInsert []dal.Record
		RecordsToUpdate []*Updates
		RecordsToDelete []*dal.Key
	}
	type args struct {
		ctx         context.Context
		tx          dal.ReadwriteTransaction
		excludeKeys []*dal.Key
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		assertErr assert.ErrorAssertionFunc
	}{
		{
			name: "nil",
			fields: fields{
				recordsToInsert: nil,
				RecordsToUpdate: nil,
				RecordsToDelete: nil,
			},
			assertErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &WithRecordChanges{
				recordsToInsert: tt.fields.recordsToInsert,
				RecordsToUpdate: tt.fields.RecordsToUpdate,
				RecordsToDelete: tt.fields.RecordsToDelete,
			}
			err := v.ApplyChanges(tt.args.ctx, tt.args.tx, tt.args.excludeKeys...)
			tt.assertErr(t, err)
		})
	}
}

func TestWithRecordChanges_QueueForInsert(t *testing.T) {
	type fields struct {
		recordsToInsert []dal.Record
	}
	type args struct {
		records []dal.Record
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "nil",
			fields: fields{
				recordsToInsert: nil,
			},
			args: args{
				records: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &WithRecordChanges{
				recordsToInsert: tt.fields.recordsToInsert,
			}
			v.QueueForInsert(tt.args.records...)
		})
	}
}

func TestWithRecordChanges_RecordsToInsert(t *testing.T) {
	type fields struct {
		recordsToInsert []dal.Record
		RecordsToUpdate []*Updates
		RecordsToDelete []*dal.Key
	}
	tests := []struct {
		name   string
		fields fields
	}{
		{
			name: "nil",
			fields: fields{
				recordsToInsert: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &WithRecordChanges{
				recordsToInsert: tt.fields.recordsToInsert,
				RecordsToUpdate: tt.fields.RecordsToUpdate,
				RecordsToDelete: tt.fields.RecordsToDelete,
			}
			assert.Equalf(t, tt.fields.recordsToInsert, v.RecordsToInsert(), "RecordsToInsert()")
		})
	}
}
