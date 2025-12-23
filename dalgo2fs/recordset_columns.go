package dalgo2fs

import (
	"time"

	"github.com/dal-go/dalgo/recordset"
)

const (
	ColumnFileName     = "File.Name"
	ColumnIsDir        = "File.IsDir"
	ColumnFileSize     = "File.Size"
	ColumnFileExt      = "File.Ext"
	ColumnFileModified = "File.Modified"
)

func NewFileNameColumn() recordset.Column[string] {
	return recordset.NewTypedColumn(ColumnFileName, "")
}

func NewIsDirColumn() recordset.Column[bool] {
	return recordset.NewBoolColumn(ColumnIsDir)
}

func NewFileExtColumn() recordset.Column[string] {
	return recordset.NewBitmapColumn[string](ColumnFileExt, 0, func() string {
		return ""
	})
}

func NewFileSizeColumn() recordset.Column[int64] {
	return recordset.NewTypedColumn[int64](ColumnFileSize, 0)
}

func NewFileModifiedColumn() recordset.Column[time.Time] {
	return recordset.NewTypedColumn(ColumnFileModified, time.Time{})
}

func NewFileInfoColumns() []recordset.Column[any] {
	return []recordset.Column[any]{
		recordset.UntypedCol(NewFileNameColumn()),
		recordset.UntypedCol(NewFileSizeColumn()),
		recordset.UntypedCol(NewFileExtColumn()),
		recordset.UntypedCol(NewFileModifiedColumn()),
	}
}
