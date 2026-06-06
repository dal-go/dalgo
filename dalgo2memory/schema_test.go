package dalgo2memory

import (
	"context"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/update"
	"github.com/stretchr/testify/require"
)

type user struct {
	Name string
	Role string
}

func readAll(t *testing.T, reader dal.RecordsReader) []dal.Record {
	t.Helper()
	var records []dal.Record
	for {
		record, err := reader.Next()
		if err != nil {
			require.ErrorIs(t, err, dal.ErrNoMoreRecords)
			return records
		}
		records = append(records, record)
	}
}

func TestNewDBIgnoresNilOption(t *testing.T) {
	db := NewDB(nil, WithSchema(false, WithCollection[user]("users", nil)), nil).(*database)
	require.NotNil(t, db.schema)
	require.Contains(t, db.schema.collections, "users")
}

func TestSchemaTypedQueryResults(t *testing.T) {
	ctx := context.Background()
	db := NewDB(WithSchema(false,
		WithCollection[user]("users", func() *user { return &user{Role: "member"} }),
		WithCollection[thing]("things", nil),
	)).(*database)

	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("users", "u1"), &user{Name: "Alice", Role: "admin"})))
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("users", "u2"), &user{Name: "Bob"})))

	q := dal.From(dal.NewRootCollectionRef("users", "")).NewQuery().SelectKeysOnly(reflect.String)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)

	records := readAll(t, reader)
	require.Len(t, records, 2)
	for _, record := range records {
		data, ok := record.Data().(*user)
		require.True(t, ok, "record data should be of the registered concrete type *user")
		require.NotEmpty(t, data.Name)
		// Bob was stored without a Role; the JSON round-trip leaves it empty,
		// confirming stored data wins over factory defaults.
		if data.Name == "Bob" {
			require.Empty(t, data.Role)
		}
	}
}

func TestSchemaNilFactoryUsesZeroValue(t *testing.T) {
	ctx := context.Background()
	db := NewDB(WithSchema(false,
		WithCollection[thing]("things", nil),
	)).(*database)

	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("things", "t1"), &thing{Name: "x", Count: 7})))

	q := dal.From(dal.NewRootCollectionRef("things", "")).NewQuery().SelectKeysOnly(reflect.String)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)

	records := readAll(t, reader)
	require.Len(t, records, 1)
	data, ok := records[0].Data().(*thing)
	require.True(t, ok)
	require.Equal(t, "x", data.Name)
	require.Equal(t, 7, data.Count)
}

func TestSchemaUndefinedCollectionErrors(t *testing.T) {
	ctx := context.Background()
	db := NewDB(WithSchema(false,
		WithCollection[user]("users", nil),
	)).(*database)
	key := dal.NewKeyWithID("orphans", "o1")

	// All write/read operations on an undefined collection are rejected.
	require.Error(t, db.Set(ctx, dal.NewRecordWithData(key, &thing{Name: "x"})))
	require.Error(t, db.Insert(ctx, dal.NewRecordWithData(key, &thing{Name: "x"})))
	require.Error(t, db.Get(ctx, dal.NewRecordWithData(key, &thing{})))
	require.Error(t, db.Update(ctx, key, []update.Update{update.ByFieldName("Name", "x")}))

	// Nothing was written despite the Set/Insert attempts.
	require.Empty(t, db.collections["orphans"])

	// Queries against an undefined collection error too.
	q := dal.From(dal.NewRootCollectionRef("orphans", "")).NewQuery().SelectKeysOnly(reflect.String)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.Nil(t, reader)
	require.Error(t, err)
}

func TestSchemaRejectsUndefinedFieldsOnWrite(t *testing.T) {
	ctx := context.Background()
	db := NewDB(WithSchema(false,
		WithCollection[user]("users", nil),
	)).(*database)
	key := dal.NewKeyWithID("users", "u1")

	// A field that does not exist on the user type must be rejected.
	bad := dal.NewRecordWithData(key, map[string]any{"Name": "Alice", "Unknown": 1})
	require.Error(t, db.Set(ctx, bad))
	require.Error(t, db.Insert(ctx, bad))
	require.Empty(t, db.collections["users"])

	// A payload that only uses defined fields is accepted.
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(key, map[string]any{"Name": "Alice", "Role": "admin"})))

	// Updating an undefined field is rejected; the stored record is unchanged.
	require.Error(t, db.Update(ctx, key, []update.Update{update.ByFieldName("Unknown", 1)}))
	var got user
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	require.Equal(t, user{Name: "Alice", Role: "admin"}, got)

	// Updating a defined field succeeds.
	require.NoError(t, db.Update(ctx, key, []update.Update{update.ByFieldName("Role", "member")}))
	require.NoError(t, db.Get(ctx, dal.NewRecordWithData(key, &got)))
	require.Equal(t, "member", got.Role)
}

func TestSchemaQueryMalformedStoredData(t *testing.T) {
	ctx := context.Background()
	db := NewDB(WithSchema(false,
		WithCollection[thing]("things", nil),
	)).(*database)
	key := dal.NewKeyWithID("things", "bad")
	// Decodes fine into map[string]any, but Count("x") fails to decode into thing.
	db.collections["things"] = map[string][]byte{keyID(key): []byte(`{"Count":"x"}`)}

	q := dal.From(dal.NewRootCollectionRef("things", "")).NewQuery().SelectKeysOnly(reflect.String)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.Nil(t, reader)
	require.Error(t, err)
}

func TestSchemaAllowUndefinedFallsBack(t *testing.T) {
	ctx := context.Background()
	db := NewDB(WithSchema(true,
		WithCollection[user]("users", nil),
	)).(*database)
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("orphans", "o1"), &thing{Name: "x"})))

	q := dal.From(dal.NewRootCollectionRef("orphans", "")).NewQuery().SelectKeysOnly(reflect.String)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	records := readAll(t, reader)
	require.Len(t, records, 1)
	// No factory for an undefined collection => keys-only record (nil data).
	require.Nil(t, records[0].Data())
}

func TestSchemaIntoRecordTakesPrecedence(t *testing.T) {
	ctx := context.Background()
	db := NewDB(WithSchema(false,
		WithCollection[user]("users", nil),
	)).(*database)
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("users", "u1"), &user{Name: "Alice"})))

	q := dal.From(dal.NewRootCollectionRef("users", "")).NewQuery().SelectIntoRecord(func() dal.Record {
		return dal.NewRecordWithIncompleteKey("users", reflect.String, &map[string]any{})
	})
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	records := readAll(t, reader)
	require.Len(t, records, 1)
	_, isUser := records[0].Data().(*user)
	require.False(t, isUser, "explicit SelectIntoRecord should override the schema factory")
}

func TestNoSchemaUnaffected(t *testing.T) {
	ctx := context.Background()
	db := NewDB().(*database)
	require.Nil(t, db.schema)
	require.NoError(t, db.Set(ctx, dal.NewRecordWithData(dal.NewKeyWithID("users", "u1"), &user{Name: "Alice"})))

	q := dal.From(dal.NewRootCollectionRef("users", "")).NewQuery().SelectKeysOnly(reflect.String)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, q)
	require.NoError(t, err)
	records := readAll(t, reader)
	require.Len(t, records, 1)
	require.Nil(t, records[0].Data())
}
