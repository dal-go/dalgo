package mock_update

import (
	"go.uber.org/mock/gomock"
	"testing"
)

func TestMockUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	update := NewMockUpdate(ctrl)
	update.EXPECT().FieldPath().Return([]string{"foo", "bar"}).Times(1)
	update.FieldPath()
	update.EXPECT().Value().Return("foo").Times(1)
	update.Value()
	update.EXPECT().FieldName().Return("bar").Times(1)
	update.FieldName()
}
