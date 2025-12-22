package dal

// Record is a gateway to a database record.
type Record interface {
	// Key keeps fields  of an entity and an ID within that table or a chain of nested keys
	Key() *Key

	// Error keeps an error for the last operation on the record. Not found is not treated as an error
	Error() error

	// Exists indicates if record was found in database. Throws panic if called before a `Get` or `Set`.
	Exists() bool

	// SetError sets error relevant to specific record. Intended to be used only by DALgo DB drivers.
	// Returns the record itself for convenience.
	SetError(err error) Record

	// Data returns record data (without ID/key).
	// Requires either record to be created by NewRecordWithData()
	// or DataTo() to be called first, otherwise panics.
	Data() any

	// HasChanged & MarkAsChanged are methods of convenience
	HasChanged() bool

	// MarkAsChanged & HasChanged are methods of convenience
	MarkAsChanged()
}
