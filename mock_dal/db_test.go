package mock_dal

import (
	"go.uber.org/mock/gomock"
	"testing"
)

func TestNewMockDB(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockDB := NewMockDB(ctrl)
	if mockDB == nil {
		t.Errorf("NewMockDB() = nil, want dal.DB")
	}
}
