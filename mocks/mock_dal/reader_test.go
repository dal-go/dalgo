package mock_dal

import (
	"github.com/dal-go/dalgo/dal"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestNewMockReader(t *testing.T) {
	ctrl := gomock.NewController(t)
	readerMock := NewMockReader(ctrl)

	readerMock.EXPECT().Close().Return(nil).AnyTimes()

	var i int

	readerMock.EXPECT().Next().DoAndReturn(func() (dal.Record, error) {
		i++
		key := dal.NewKeyWithID("tests", i)
		return dal.NewRecord(key), nil
	})

	var reader dal.Reader = readerMock
	record, err := reader.Next()
	if err != nil {
		t.Errorf("reader.Next(): expected err == nil, got %v", err)
	}
	if record == nil {
		t.Fatal("reader.Next(): expected record != nil")
	}
	if key := record.Key(); key == nil {
		t.Error("reader.Next(): expected key != nil")
	}
	if err = reader.Close(); err != nil {
		t.Fatalf("failed to close mock reader: %v", err)
	}
}
