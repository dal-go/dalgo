package mock_dal

import (
	"testing"

	"github.com/dal-go/dalgo/dal"
	"go.uber.org/mock/gomock"
)

func TestNewMockReader(t *testing.T) {
	ctrl := gomock.NewController(t)
	readerMock := NewMockRecordsReader(ctrl)

	readerMock.EXPECT().Close().Return(nil).AnyTimes()

	// Cover Cursor method as well
	readerMock.EXPECT().Cursor().Return("", nil)

	var i int

	readerMock.EXPECT().Next().DoAndReturn(func() (dal.Record, error) {
		i++
		key := dal.NewKeyWithID("tests", i)
		return dal.NewRecord(key), nil
	})

	var reader dal.RecordsReader = readerMock
	// Call Cursor to cover it
	if cursor, err := reader.Cursor(); err != nil || cursor != "" {
		t.Errorf("reader.Cursor(): expected \"\", nil; got %q, %v", cursor, err)
	}
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

func TestMockReader_Cursor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	readerMock := NewMockReader(ctrl)
	readerMock.EXPECT().Cursor().Return("cursor-token", nil)

	cursor, err := readerMock.Cursor()
	if err != nil {
		t.Fatalf("expected no error from Cursor, got: %v", err)
	}
	if cursor != "cursor-token" {
		t.Fatalf("expected cursor-token, got: %s", cursor)
	}
}
