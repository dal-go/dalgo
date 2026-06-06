package dalgo2memory

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/update"
	"github.com/stretchr/testify/require"
)

// profile is a record type with a nested slice and struct, used to prove the
// Serialized engine's ref-breaking fidelity (mutating a caller value after a
// write, or one decoded read, never affects stored data or another read).
type profile struct {
	Name    string
	Tags    []string
	Address address
}

type address struct {
	City string
}

// TestSerialized_StoredAsDecodableBytes verifies
// serialized-storage#ac:stored-as-decodable-bytes: a record Set then Get
// returns data equal to what was written, and the stored form is a byte buffer
// that, when decoded directly, reproduces that same data.
func TestSerialized_StoredAsDecodableBytes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("users", "u1")
	written := &user{Name: "Alice", Role: "admin"}
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, written)))

	// Get reconstructs equal data.
	var got user
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	require.Equal(t, *written, got)

	// The stored form is a byte buffer, not the original value; decoding those
	// bytes directly yields the same data.
	engine, ok := db.collections["users"].(*serializedEngine)
	require.True(t, ok)
	stored, ok := engine.records[keyID(key)]
	require.True(t, ok)
	require.IsType(t, []byte{}, stored)
	var decoded user
	require.NoError(t, json.Unmarshal(stored, &decoded))
	require.Equal(t, *written, decoded)
}

// TestSerialized_DefaultAndSchemalessCapable verifies
// serialized-storage#ac:serialized-is-default-and-schemaless-capable: an
// unregistered (schemaless) collection and a WithSerializedStorage() collection
// both operate through the Serialized engine and succeed.
func TestSerialized_DefaultAndSchemalessCapable(t *testing.T) {
	t.Parallel()

	// Compile-time conformance guard (mirrors the assertion in serialized.go).
	var _ storageEngine = (*serializedEngine)(nil)

	ctx := context.Background()
	db := NewDB(WithSchema(true,
		WithCollection[user]("users", nil, WithSerializedStorage()),
	)).(*database)

	// Schema-typed collection selected via WithSerializedStorage().
	typedKey := dal.NewKeyWithID("users", "u1")
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(typedKey, &user{Name: "Alice", Role: "admin"})))
	_, typedIsSerialized := db.collections["users"].(*serializedEngine)
	require.True(t, typedIsSerialized)
	var typedGot user
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(typedKey, &typedGot)))
	require.Equal(t, "Alice", typedGot.Name)

	// Unregistered, schemaless collection (allowed by allowUndefinedCollections).
	schemalessKey := dal.NewKeyWithID("ad-hoc", "x1")
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(schemalessKey, map[string]any{"anything": "goes"})))
	engine, schemalessIsSerialized := db.collections["ad-hoc"].(*serializedEngine)
	require.True(t, schemalessIsSerialized)
	require.Nil(t, engine.factory, "schemaless collection has no record factory")
	var schemalessGot map[string]any
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(schemalessKey, &schemalessGot)))
	require.Equal(t, "goes", schemalessGot["anything"])
}

// TestSerialized_MutationAfterWriteIsolated verifies
// serialized-storage#ac:mutation-after-write-isolated: mutating a nested field
// of the caller value AFTER Set returns does not affect stored data; a fresh
// Get reflects the write-time value.
func TestSerialized_MutationAfterWriteIsolated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("profiles", "p1")

	written := &profile{Name: "Alice", Tags: []string{"a", "b"}, Address: address{City: "Paris"}}
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, written)))

	// Mutate nested slice element and nested struct field after Set returns.
	written.Tags[0] = "MUTATED"
	written.Address.City = "MUTATED"

	var got profile
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	require.Equal(t, []string{"a", "b"}, got.Tags)
	require.Equal(t, "Paris", got.Address.City)
}

// TestSerialized_TwoReadsIndependent verifies
// serialized-storage#ac:two-reads-independent: reading one stored record into
// two separate targets and mutating one leaves the other unchanged.
func TestSerialized_TwoReadsIndependent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("profiles", "p1")
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key,
		&profile{Name: "Alice", Tags: []string{"a", "b"}, Address: address{City: "Paris"}})))

	var first, second profile
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &first)))
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &second)))

	first.Tags[0] = "MUTATED"
	first.Address.City = "MUTATED"

	require.Equal(t, []string{"a", "b"}, second.Tags)
	require.Equal(t, "Paris", second.Address.City)
}

// TestSerialized_NonSerializableWriteErrors verifies
// serialized-storage#ac:non-serializable-write-errors: writing a record with a
// non-serializable value returns a descriptive error, sets it on the record,
// and leaves no entry for that id.
func TestSerialized_NonSerializableWriteErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("bad", "ch")
	record := dal.NewRecordWithData(key, map[string]any{"ch": make(chan int)})

	err := db.Set(ctx, record)
	require.Error(t, err)
	require.ErrorContains(t, err, "json")
	require.Equal(t, err, record.Error(), "the error is set on the record")

	exists, existsErr := db.Exists(ctx, key)
	require.NoError(t, existsErr)
	require.False(t, exists, "no entry exists for the id after a failed write")
}

// TestSerialized_UnknownFieldRejectedWhenTyped verifies
// serialized-storage#ac:unknown-field-rejected-when-typed: a write carrying a
// field undefined on the registered type is rejected with a descriptive error
// naming the collection; the same data on a schemaless collection succeeds.
func TestSerialized_UnknownFieldRejectedWhenTyped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	data := map[string]any{"Name": "Alice", "Undefined": 1}

	// Schema-typed collection rejects the unknown field, naming the collection.
	typedDB := NewDB(WithSchema(false, WithCollection[user]("users", nil))).(*database)
	typedKey := dal.NewKeyWithID("users", "u1")
	err := typedDB.Set(ctx, dal.NewRecordWithData(typedKey, data))
	require.Error(t, err)
	require.ErrorContains(t, err, "users", "error names the collection")
	require.Empty(t, typedDB.collections["users"].(*serializedEngine).records)

	// Schemaless collection accepts the same data.
	schemalessDB := NewDB().(*database)
	schemalessKey := dal.NewKeyWithID("users", "u1")
	require.NoError(t, schemalessDB.Set(ctx, dal.NewRecordWithData(schemalessKey, data)))
	var got map[string]any
	require.NoError(t, schemalessDB.Get(ctx, dal.NewRecordWithData(schemalessKey, &got)))
	require.EqualValues(t, 1, got["Undefined"])
}

// TestSerialized_InsertDuplicateErrors verifies
// serialized-storage#ac:insert-duplicate-errors: Insert on an existing id
// returns an "already exists" error and leaves the existing record unchanged,
// while Set for that id overwrites.
func TestSerialized_InsertDuplicateErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("users", "u1")
	require.NoError(t, db.Insert(ctx, dal.NewRecordWithData(key, &user{Name: "Alice", Role: "admin"})))

	// Insert on the existing id fails with an "already exists" error.
	err := db.Insert(ctx, dal.NewRecordWithData(key, &user{Name: "Bob", Role: "member"}))
	require.Error(t, err)
	require.ErrorContains(t, err, "already exists")

	// The existing record is unchanged.
	var got user
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	require.Equal(t, user{Name: "Alice", Role: "admin"}, got)

	// Set for that id overwrites successfully.
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, &user{Name: "Bob", Role: "member"})))
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	require.Equal(t, user{Name: "Bob", Role: "member"}, got)
}

// TestSerialized_GetAbsentNotFound verifies
// serialized-storage#ac:get-absent-not-found: Get on an absent id returns a
// not-found error set on the record, and Exists reports false.
func TestSerialized_GetAbsentNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB().(*database)
	key := dal.NewKeyWithID("users", "absent")

	record := dal.NewRecordWithData(key, &user{})
	err := db.Get(ctx, record)
	require.Error(t, err)
	require.True(t, dal.IsNotFound(err))
	// The not-found error is set on the record: the record reports it does not
	// exist (dal.Record.Error() normalizes a not-found error to a false Exists).
	require.False(t, record.Exists(), "the not-found error is set on the record")

	exists, existsErr := db.Exists(ctx, key)
	require.NoError(t, existsErr)
	require.False(t, exists)
}

// TestSerialized_QueryDecodesRows verifies
// serialized-storage#ac:query-decodes-rows: an equality-filtered query that
// materializes into a typed target yields decoded rows for filtering and
// decoded typed result records that do not share references.
func TestSerialized_QueryDecodesRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB(WithSchema(false,
		WithCollection[profile]("profiles", nil),
	)).(*database)

	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("profiles", "p1"),
		&profile{Name: "shared", Tags: []string{"x"}, Address: address{City: "Paris"}})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("profiles", "p2"),
		&profile{Name: "shared", Tags: []string{"y"}, Address: address{City: "Lyon"}})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("profiles", "p3"),
		&profile{Name: "other", Tags: []string{"z"}, Address: address{City: "Nice"}})))

	q := dal.From(dal.NewRootCollectionRef("profiles", "")).NewQuery().
		WhereField("Name", dal.Equal, "shared").
		SelectIntoRecord(func() dal.Record {
			return dal.NewRecordWithIncompleteKey("profiles", reflect.String, &profile{})
		})
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	records := readAll(t, reader)

	// The equality filter, evaluated over decoded map rows, selects only the two
	// "shared" records.
	require.Len(t, records, 2)
	first, ok := records[0].Data().(*profile)
	require.True(t, ok)
	second, ok := records[1].Data().(*profile)
	require.True(t, ok)
	require.Equal(t, "shared", first.Name)
	require.Equal(t, "shared", second.Name)

	// Two result records do not share references: mutating one's nested slice
	// leaves the other unchanged.
	require.NotSame(t, first, second)
	first.Tags[0] = "MUTATED"
	require.NotEqual(t, "MUTATED", second.Tags[0])
}

// TestSerialized_UpdateAppliesAndRevalidates verifies
// serialized-storage#ac:update-applies-and-revalidates: Update applies a
// defined-field change (read-modify-write, persisted); an update introducing an
// undefined field is rejected with a descriptive error; Update on an absent id
// returns not-found and stores nothing.
func TestSerialized_UpdateAppliesAndRevalidates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB(WithSchema(false,
		WithCollection[user]("users", nil),
	)).(*database)
	key := dal.NewKeyWithID("users", "u1")
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, &user{Name: "Alice", Role: "admin"})))

	// A defined-field change is read-modified-written and persisted; the
	// untouched field (Name) survives the read-modify-write.
	require.NoError(t, db.Update(ctx, key, []update.Update{update.ByFieldName("Role", "member")}))
	var got user
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	require.Equal(t, user{Name: "Alice", Role: "member"}, got)

	// An update introducing an undefined field is rejected, naming the
	// collection; the stored record is unchanged.
	err := db.Update(ctx, key, []update.Update{update.ByFieldName("Undefined", 1)})
	require.Error(t, err)
	require.ErrorContains(t, err, "users")
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	require.Equal(t, user{Name: "Alice", Role: "member"}, got)

	// Update on an absent id returns not-found and stores nothing.
	absentKey := dal.NewKeyWithID("users", "absent")
	err = db.Update(ctx, absentKey, []update.Update{update.ByFieldName("Role", "x")})
	require.Error(t, err)
	require.True(t, dal.IsNotFound(err))
	exists, existsErr := db.Exists(ctx, absentKey)
	require.NoError(t, existsErr)
	require.False(t, exists)
}
