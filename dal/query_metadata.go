package dal

import (
	"fmt"
	"github.com/dal-go/dalgo/datarights"
)

// QueryMetadataProvider is an optional reader capability. Metadata must be
// available before Next; existing Reader interfaces and providers are unchanged.
// Providers must freeze the planned rights inventory before output and report
// actual used source IDs independently of row counts.
type QueryMetadataProvider interface {
	QueryMetadata() datarights.QueryMetadata
}

// ReadQueryMetadata returns a detached snapshot and whether the reader exposes
// this optional capability. A false result means metadata was not provided.
func ReadQueryMetadata(reader Reader) (datarights.QueryMetadata, bool) {
	if provider, ok := reader.(QueryMetadataProvider); ok {
		return provider.QueryMetadata().Clone(), true
	}
	return datarights.QueryMetadata{}, false
}

// WithRecordsQueryMetadata attaches a detached, fixed snapshot to a reader.
// Cursor, Close and Next retain the wrapped reader's behavior.
func WithRecordsQueryMetadata(reader RecordsReader, metadata datarights.QueryMetadata) RecordsReader {
	return &recordsMetadataReader{RecordsReader: reader, metadata: metadata.Clone()}
}

// WithRecordsetQueryMetadata attaches the same optional capability to a
// columnar reader without requiring any provider interface implementation.
func WithRecordsetQueryMetadata(reader RecordsetReader, metadata datarights.QueryMetadata) RecordsetReader {
	return &recordsetMetadataReader{RecordsetReader: reader, metadata: metadata.Clone()}
}

type recordsMetadataReader struct {
	RecordsReader
	metadata datarights.QueryMetadata
}

func (r *recordsMetadataReader) QueryMetadata() datarights.QueryMetadata { return r.metadata.Clone() }

type recordsetMetadataReader struct {
	RecordsetReader
	metadata datarights.QueryMetadata
}

func (r *recordsetMetadataReader) QueryMetadata() datarights.QueryMetadata { return r.metadata.Clone() }

// Generic transforms currently cannot preflight a complete rights inventory.
// Refuse a metadata-bearing source rather than silently dropping its evidence.
func requireUnannotatedQueryInput(reader Reader) error {
	if metadata, ok := ReadQueryMetadata(reader); ok && (metadata.SourceRights != nil || metadata.UsedSourceIDs != nil) {
		return fmt.Errorf("%w: source_rights: generic query transforms cannot preflight source rights; use a metadata-aware provider executor", ErrNotSupported)
	}
	return nil
}
