package access

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// Issue 148: an allow-list that cannot be projected (a wildcard such as
// public_*) used to redact by the name a column comes back under. Selecting a
// hidden field under an alias the list allows (passwordHash AS public_hash)
// therefore returned the hidden value. A selection whose every column was
// refused was rewritten to an empty column list, which adapters read as
// SELECT *. Both have to fail closed, and an allowed field has to come back
// under the alias the caller gave it.

func issue148Policy(fields ...string) Policy {
	return MustPolicy("fields", Collection("users", Allow(Query, "list").Fields(fields...)))
}

func issue148Column(alias, field string) dal.Column {
	return dal.Column{Alias: alias, Expression: dal.Field(field)}
}

func issue148Memory(t *testing.T) dal.DB {
	t.Helper()
	ctx := context.Background()
	raw := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	data := map[string]any{"id": "u1", "public_bio": "hello", "passwordHash": "h1"}
	if err := raw.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("users", "u1"), &data))
	}); err != nil {
		t.Fatal(err)
	}
	return raw
}

func issue148Rows(t *testing.T, session dal.ReadSession, query dal.Query) ([]map[string]any, error) {
	t.Helper()
	reader, err := session.ExecuteQueryToRecordsReader(context.Background(), query)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	for {
		rec, err := reader.Next()
		if errors.Is(err, dal.ErrNoMoreRecords) {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		switch data := rec.Data().(type) {
		case map[string]any:
			rows = append(rows, data)
		case *map[string]any:
			rows = append(rows, *data)
		default:
			return nil, fmt.Errorf("row data %T", rec.Data())
		}
	}
}

func TestIssue148AliasDoesNotBypassAllowList(t *testing.T) {
	stored := map[string]any{"id": "u1", "public_bio": "hello", "passwordHash": "h1"}
	lists := map[string][]string{
		"enumerable": {"id", "public_bio"},
		"wildcard":   {"id", "public_*"},
	}
	hiddenAsAllowed := usersQuery().SelectColumns(issue148Column("public_hash", "passwordHash"))
	hiddenAsExactAllowed := usersQuery().SelectColumns(issue148Column("id", "passwordHash"))
	onlyHidden := usersQuery().SelectColumns(dal.Column{Expression: dal.Field("passwordHash")})
	twoHidden := usersQuery().SelectColumns(
		dal.Column{Expression: dal.Field("passwordHash")},
		issue148Column("public_hash", "secret"),
	)
	whereHidden := usersQuery().WhereField("passwordHash", dal.Equal, "h1").SelectColumns(dal.Column{Expression: dal.Field("id")})
	orderHidden := usersQuery().OrderBy(dal.AscendingField("passwordHash")).SelectColumns(dal.Column{Expression: dal.Field("id")})
	sumHidden := usersQuery().SelectColumns(dal.SumAs(dal.Field("passwordHash"), "public_hash"))

	for name, fields := range lists {
		policy := issue148Policy(fields...)
		t.Run(name, func(t *testing.T) {
			for _, c := range []struct {
				name   string
				query  dal.StructuredQuery
				column string
			}{
				{"hidden field under an alias the list allows", hiddenAsAllowed, "passwordHash"},
				{"hidden field under an allowed field's exact name", hiddenAsExactAllowed, "passwordHash"},
				{"every selected column refused", onlyHidden, "passwordHash"},
				{"every selected column refused, first one named", twoHidden, "passwordHash"},
				{"filter on a hidden field", whereHidden, "passwordHash"},
				{"order by a hidden field", orderHidden, "passwordHash"},
				{"aggregate of a hidden field under an allowed alias", sumHidden, "passwordHash"},
			} {
				t.Run(c.name, func(t *testing.T) {
					for _, path := range []string{"records", "recordset"} {
						t.Run(path, func(t *testing.T) {
							wrapped := &countingSession{}
							session := SecureReadSession(wrapped, policy)
							var err error
							if path == "records" {
								_, err = session.ExecuteQueryToRecordsReader(context.Background(), c.query)
							} else {
								_, err = session.ExecuteQueryToRecordsetReader(context.Background(), c.query)
							}
							var denied *DeniedError
							if !errors.As(err, &denied) || wrapped.reads != 0 {
								t.Fatalf("error = %v, reads = %d, want a denial before the adapter", err, wrapped.reads)
							}
							// The decision names the source field. An allow-list
							// match on the alias alone would have allowed the query.
							if denied.Decision.Code != CodeColumnDenied || !reflect.DeepEqual(denied.Decision.Columns, [][]string{{c.column}}) {
								t.Fatalf("decision = code %s columns %v", denied.Decision.Code, denied.Decision.Columns)
							}
						})
					}
				})
			}

			// An adapter that honours aliases would return the hidden value
			// under the allowed name. The session must not ask it to.
			leaky := &projectingSession{stored: stored}
			if _, err := issue148Rows(t, SecureReadSession(leaky, policy), hiddenAsAllowed); !errors.Is(err, ErrAccessDenied) || len(leaky.seen) != 0 {
				t.Fatalf("alias bypass: error = %v, queries sent = %d", err, len(leaky.seen))
			}
		})
	}
}

func TestIssue148AllRefusedProjectionDoesNotWiden(t *testing.T) {
	sets := fieldList(t, "id", "public_bio")
	onlyHidden := usersQuery().SelectColumns(dal.Column{Expression: dal.Field("passwordHash")})
	projection := projectQuery(onlyHidden, sets)
	if projection.status == queryProjectionApplied && len(projection.query.Columns()) == 0 {
		t.Fatal("an all-refused selection was rewritten to an empty column list, which adapters read as SELECT *")
	}
	wildcard := fieldList(t, "id", "public_*")
	widened := projectQuery(onlyHidden, wildcard)
	if widened.status == queryProjectionApplied && len(widened.query.Columns()) == 0 {
		t.Fatal("a non-enumerable all-refused selection was rewritten to SELECT *")
	}
	// Nothing left after a wildcard exclusion is the empty-projection case the
	// session denies, rather than a column list adapters treat as every field.
	excluded := usersQuery().SelectColumns(dal.AllColumnsExcept("id", "public_bio"))
	if got := projectQuery(excluded, sets); got.status != queryProjectionEmpty {
		t.Fatalf("status = %v, want an empty projection", got.status)
	}

	raw := issue148Memory(t)
	for name, fields := range map[string][]string{
		"enumerable": {"id", "public_bio"},
		"wildcard":   {"id", "public_*"},
	} {
		t.Run(name, func(t *testing.T) {
			secured := MustSecureDB(raw, WithDatabasePolicies(issue148Policy(fields...)))
			rows, err := issue148Rows(t, secured, onlyHidden)
			if !errors.Is(err, ErrAccessDenied) {
				t.Fatalf("error = %v, rows = %v, want a denial and no rows", err, rows)
			}
			for _, row := range rows {
				if _, leaked := row["passwordHash"]; leaked {
					t.Fatalf("hidden field returned: %v", row)
				}
				if _, widenedID := row["id"]; widenedID {
					t.Fatalf("refused selection widened to allowed fields: %v", row)
				}
			}
		})
	}
}

func TestIssue148AllowedFieldComesBackUnderItsAlias(t *testing.T) {
	raw := issue148Memory(t)
	lists := map[string][]string{
		"enumerable": {"id", "public_bio"},
		"wildcard":   {"id", "public_*"},
	}
	queries := map[string]dal.StructuredQuery{
		"alias the list allows":    usersQuery().SelectColumns(issue148Column("public_id", "id")),
		"alias the list refuses":   usersQuery().SelectColumns(issue148Column("passwordHash", "id")),
		"field under its own name": usersQuery().SelectColumns(dal.Column{Expression: dal.Field("id")}),
		"two allowed fields": usersQuery().SelectColumns(
			dal.Column{Expression: dal.Field("id")},
			issue148Column("bio", "public_bio"),
		),
	}
	want := map[string]map[string]any{
		"alias the list allows":    {"public_id": "u1"},
		"alias the list refuses":   {"passwordHash": "u1"},
		"field under its own name": {"id": "u1"},
		"two allowed fields":       {"id": "u1", "bio": "hello"},
	}
	for list, fields := range lists {
		secured := MustSecureDB(raw, WithDatabasePolicies(issue148Policy(fields...)))
		for name, query := range queries {
			t.Run(list+" "+name, func(t *testing.T) {
				rows, err := issue148Rows(t, secured, query)
				if err != nil {
					t.Fatal(err)
				}
				if len(rows) != 1 || !reflect.DeepEqual(rows[0], want[name]) {
					t.Fatalf("rows = %v, want [%v]", rows, want[name])
				}
			})
		}
	}

	// No column list. An enumerable policy is projected to the allowed fields.
	// A wildcard policy cannot be projected: the adapter returns the whole row
	// and redaction removes passwordHash by the field's own name. Neither path
	// returns the hidden field.
	intoRow := func() record.Record {
		return record.NewRecordWithData(record.NewKeyWithID("users", ""), &map[string]any{})
	}
	for _, fields := range lists {
		secured := MustSecureDB(raw, WithDatabasePolicies(issue148Policy(fields...)))
		rows, err := issue148Rows(t, secured, usersQuery().SelectIntoRecord(intoRow))
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || !reflect.DeepEqual(rows[0], map[string]any{"id": "u1", "public_bio": "hello"}) {
			t.Fatalf("fields %v: row = %v", fields, rows)
		}
	}
}
