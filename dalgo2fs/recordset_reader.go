package dalgo2fs

import (
	"os"
	"path"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
)

var _ dal.RecordsetReader = (*dirReader)(nil)

func NewDirReader(name string, columns ...recordset.Column[any]) (reader dal.RecordsetReader, err error) {
	var entries []os.DirEntry
	if entries, err = os.ReadDir(name); err != nil {
		return
	}
	r := dirReader{
		entries: entries,
		rs: recordset.NewColumnarRecordset(
			columns...,
		),
	}
	nameCol := r.rs.GetColumnByName(ColumnFileName)
	if nameCol != nil {
		r.nameCol = nameCol.(recordset.UntypedColWrapper[string]).TypedColumn()
	}
	extCol := r.rs.GetColumnByName(ColumnFileExt)
	if extCol != nil {
		r.extCol = extCol.(recordset.UntypedColWrapper[string]).TypedColumn()
	}
	sizeCol := r.rs.GetColumnByName(ColumnFileSize)
	if sizeCol != nil {
		r.sizeCol = sizeCol.(recordset.UntypedColWrapper[int64]).TypedColumn()
	}
	isDirCol := r.rs.GetColumnByName(ColumnIsDir)
	if isDirCol != nil {
		r.isDirCol = isDirCol.(recordset.UntypedColWrapper[bool]).TypedColumn()
	}
	modifiedCol := r.rs.GetColumnByName(ColumnFileModified)
	if modifiedCol != nil {
		r.modifiedCol = modifiedCol.(recordset.UntypedColWrapper[time.Time]).TypedColumn()
	}
	return &r, nil
}

type dirReader struct {
	rs          recordset.Recordset
	nameCol     recordset.Column[string]
	extCol      recordset.Column[string]
	sizeCol     recordset.Column[int64]
	isDirCol    recordset.Column[bool]
	modifiedCol recordset.Column[time.Time]
	nextIndex   int
	entries     []os.DirEntry
}

func (r *dirReader) Recordset() recordset.Recordset {
	return r.rs
}

func (r *dirReader) Cursor() (string, error) {
	return "", dal.ErrNotSupported
}

func (r *dirReader) Close() error {
	r.entries = nil
	r.rs = nil
	return nil
}

func (r *dirReader) Next() (row recordset.Row, rs recordset.Recordset, err error) {
	if r.nextIndex >= len(r.entries) {
		return nil, r.rs, dal.ErrNoMoreRecords
	}
	rs = r.rs
	dirEntry := r.entries[r.nextIndex]
	for dirEntry.IsDir() {
		r.nextIndex++
		if r.nextIndex >= len(r.entries) {
			return nil, r.rs, dal.ErrNoMoreRecords
		}
		dirEntry = r.entries[r.nextIndex]
	}
	row = r.rs.NewRow()
	rowIndex := r.rs.RowsCount() - 1
	columns := r.rs.Columns()
	var fi os.FileInfo
	for _, col := range columns {
		switch col.Name() {
		case ColumnFileName:
			_ = r.nameCol.SetValue(rowIndex, dirEntry.Name())
		case ColumnIsDir:
			_ = r.isDirCol.SetValue(rowIndex, dirEntry.IsDir())
		case ColumnFileExt:
			fileName := dirEntry.Name()
			ext := path.Ext(fileName)
			_ = r.extCol.SetValue(rowIndex, ext)
		case ColumnFileSize, ColumnFileModified:
			if fi == nil {
				if fi, err = dirEntry.Info(); err != nil {
					return nil, r.rs, err
				}
			}
			if col.Name() == ColumnFileSize {
				_ = r.sizeCol.SetValue(rowIndex, fi.Size())
			} else {
				_ = r.modifiedCol.SetValue(rowIndex, fi.ModTime())
			}
		}
	}
	r.nextIndex++
	return
}
