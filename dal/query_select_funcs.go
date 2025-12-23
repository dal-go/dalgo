package dal

// ReaderOption configures how SelectAll reads from the RecordsReader (e.g., limit, offset).
type ReaderOption = func(ro *ReaderOptions)

type ReaderOptions struct {
	offset int
	limit  int
}

// Offset specifies how many records to skip, if 0 - no records are skipped
func (ro *ReaderOptions) Offset() int {
	return ro.offset
}

// Limit specifies the maximum number of records to read, if 0 - unlimited
func (ro *ReaderOptions) Limit() int {
	return ro.limit
}

// WithLimit sets the maximum number of items to read.
// If limit <= 0, SelectAll reads until ErrNoMoreRecords.
func WithLimit(limit int) ReaderOption {
	return func(ro *ReaderOptions) {
		ro.limit = limit
	}
}

// WithOffset skips the first N records before collecting results in SelectAll.
// If offset <= 0, no records are skipped.
func WithOffset(offset int) ReaderOption {
	return func(ro *ReaderOptions) {
		ro.offset = offset
	}
}

func newReaderOptions(options ...ReaderOption) *ReaderOptions {
	ro := &ReaderOptions{}
	for _, o := range options {
		o(ro)
	}
	return ro
}
