package dal

import (
	"fmt"

	"github.com/dal-go/record"
)

// DocumentID is the typed record/document identity expression for ordered
// queries. Adapters map it to their native identity field instead of treating
// "__name__" as an application data field.
func DocumentID() FieldRef {
	return FieldRef{name: "__name__", isID: true}
}

// NewDocumentIDCursor validates an opaque record ID for a query ordered by
// DocumentID. It avoids adapter-specific string cursors at call sites.
func NewDocumentIDCursor(id string) (Cursor, error) {
	if err := record.ValidateStringID(id); err != nil {
		return "", fmt.Errorf("invalid document ID cursor: %w", err)
	}
	return Cursor(id), nil
}
