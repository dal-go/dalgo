package recordset

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestColumnarRecordset(t *testing.T) {
	var rs *ColumnarRecordset
	t.Run("NewColumnarRecordset", func(t *testing.T) {
		rs = NewColumnarRecordset(
			NewColumn[string]("FirstName", ""),
			NewColumn[int]("Age", 0),
			NewColumn[time.Time]("DateOfBirth", time.Time{}),
		)
		if rs == nil {
			t.Fatal("NewColumnarRecordset() returned nil")
		}
	})
	t.Run("initial RowsCount", func(t *testing.T) {
		if rowsCount := rs.RowsCount(); rowsCount > 0 {
			t.Errorf("NewColumnarRecordset() returned %d row(s), expected 0", rowsCount)
		}
	})

	rowIndex := -1

	testNewRow := func(firstName string, age int) {
		t.Run(fmt.Sprintf("NewRow[%d]", rowIndex+1), func(t *testing.T) {
			row := rs.NewRow()
			rowIndex++
			var err error
			if rowsCount := rs.RowsCount(); rowsCount != rowIndex+1 {
				t.Errorf("NewColumnarRecordset() returned %d row(s), expected 1", rowsCount)
			}

			testValue := func(colIndex int, val any) {
				t.Run(fmt.Sprintf("test column %d", colIndex), func(t *testing.T) {
					if err = row.SetValueByIndex(colIndex, val, rs); err != nil {
						t.Errorf("SetValueByIndex failed: %v", err)
					}
					if value, err := row.GetValueByIndex(colIndex, rs); err != nil {
						t.Errorf("GetValueByIndex failed: %v", err)
					} else if value != val {
						t.Errorf("GetValueByIndex returned %v, expected %v", value, val)
					}
				})
			}

			dob := time.Now().Add(-time.Hour * 24 * 365 * time.Duration(age))
			testValue(0, firstName)
			testValue(1, age)
			testValue(2, dob)
			row = rs.GetRow(rowIndex)
			data, err := row.Data(rs)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, []any{firstName, age, dob}, data)
		})
	}

	testNewRow("Anna", 19)
	testNewRow("Bob", 23)
}
