package dal

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestWithExistsPrecondition(t *testing.T) {
	preconditions := preConditions{}
	assert.False(t, preconditions.Exists())
	preCondition := WithExistsPrecondition()
	assert.NotNil(t, preCondition)
	preCondition.apply(&preconditions)
	assert.True(t, preconditions.Exists())
}

func TestWithLastUpdateTimePrecondition(t *testing.T) {
	preconditions := preConditions{}
	assert.True(t, preconditions.lastUpdateTime.IsZero())
	expected := time.Now()
	preCondition := WithLastUpdateTimePrecondition(expected)
	assert.NotNil(t, preCondition)
	preCondition.apply(&preconditions)
	assert.Equal(t, expected, preconditions.LastUpdateTime())
}

func TestGetPreconditions(t *testing.T) {
	type args struct {
		items []Precondition
	}
	now := time.Now()
	tests := []struct {
		name string
		args args
		want Preconditions
	}{
		{"empty", args{[]Precondition{}}, preConditions{}},
		{"exists", args{[]Precondition{WithExistsPrecondition()}}, preConditions{exists: true}},
		{"last_update_time", args{[]Precondition{WithLastUpdateTimePrecondition(now)}}, preConditions{lastUpdateTime: now}},
		{"all", args{[]Precondition{WithExistsPrecondition(), WithLastUpdateTimePrecondition(now)}}, preConditions{exists: true, lastUpdateTime: now}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, GetPreconditions(tt.args.items...), "GetPreconditions(%v)", tt.args.items)
		})
	}
}
