// specscore: feat-recordops/diff
package recordops

import "errors"

// ErrUnsortedInput indicates an input stream yielded a record whose ID
// is not strictly greater than the previously yielded ID from the same
// stream. Diff requires ID-sorted input streams.
var ErrUnsortedInput = errors.New("recordops: input stream not sorted ascending by ID")

// ErrDuplicateID indicates an input stream yielded two records with
// the same ID. Within a single stream, IDs must be unique.
var ErrDuplicateID = errors.New("recordops: duplicate ID in input stream")

// ErrIncomparableField indicates field comparison via reflect.DeepEqual
// panicked (e.g., a func or chan field). The panic is recovered and
// surfaced as a stream error wrapping this sentinel.
var ErrIncomparableField = errors.New("recordops: incomparable field")

// ErrInvalidArgument indicates a programmer error in calling Diff/DiffFunc
// (e.g., nil less function passed to DiffFunc).
var ErrInvalidArgument = errors.New("recordops: invalid argument")
