package dalgo2fs

import (
	"time"

	"github.com/dal-go/dalgo/recordset"
)

const (
	// ColumnFileName is the recordset column name for a directory entry's file name.
	ColumnFileName = "File.Name"
	// ColumnIsDir is the recordset column name for whether a directory entry is a directory.
	ColumnIsDir = "File.IsDir"
	// ColumnFileSize is the recordset column name for a file's size in bytes.
	ColumnFileSize = "File.Size"
	// ColumnFileExt is the recordset column name for a file name's extension.
	ColumnFileExt = "File.Ext"
	// ColumnFileModified is the recordset column name for a file's modification time.
	ColumnFileModified = "File.Modified"
)

// NewFileNameColumn returns a string column named ColumnFileName.
func NewFileNameColumn() recordset.Column[string] {
	return recordset.NewTypedColumn(ColumnFileName, "")
}

// NewIsDirColumn returns a bool column named ColumnIsDir.
func NewIsDirColumn() recordset.Column[bool] {
	return recordset.NewBoolColumn(ColumnIsDir)
}

// NewFileExtColumn returns a bitmap string column named ColumnFileExt.
func NewFileExtColumn() recordset.Column[string] {
	return recordset.NewBitmapColumn[string](ColumnFileExt, 0, func() string {
		return ""
	}, recordset.ColDbType("STRING"))
}

// NewFileSizeColumn returns an int64 column named ColumnFileSize.
func NewFileSizeColumn() recordset.Column[int64] {
	return recordset.NewTypedColumn[int64](ColumnFileSize, 0)
}

// NewFileModifiedColumn returns a time.Time column named ColumnFileModified.
func NewFileModifiedColumn() recordset.Column[time.Time] {
	return recordset.NewTypedColumn(ColumnFileModified, time.Time{})
}

// NewFileInfoColumns returns untyped columns for name, size, extension, and modification time.
// The IsDir column is not included.
func NewFileInfoColumns() []recordset.Column[any] {
	return []recordset.Column[any]{
		recordset.UntypedCol(NewFileNameColumn()),
		recordset.UntypedCol(NewFileSizeColumn()),
		recordset.UntypedCol(NewFileExtColumn()),
		recordset.UntypedCol(NewFileModifiedColumn()),
	}
}
