package access

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func TestDatabaseOptionRejectsMissingAndDuplicateCapabilities(t *testing.T) {
	var options secureDBOptions
	if err := WithEnforcementCoordinator(nil)(&options); err == nil {
		t.Fatal("nil coordinator accepted")
	}
	options.coordinator = &EnforcementCoordinator{}
	if err := WithEnforcementCoordinator(&EnforcementCoordinator{})(&options); err == nil {
		t.Fatal("duplicate coordinator accepted")
	}

	options = secureDBOptions{}
	if err := WithDatabasePolicyProvider(nil)(&options); err == nil {
		t.Fatal("nil provider accepted")
	}
	options.policyProvider = func(context.Context) ([]Policy, error) { return nil, nil }
	if err := WithDatabasePolicyProvider(func(context.Context) ([]Policy, error) { return nil, nil })(&options); err == nil {
		t.Fatal("duplicate provider accepted")
	}
}

func TestSecureDBRejectsNilOption(t *testing.T) {
	if _, err := SecureDB(dal.NewDB(&fakeDB{fakeSession: &fakeSession{}}), nil); err == nil {
		t.Fatal("nil option accepted")
	}
}

func TestBindDBVariantsAndTransactionProviderFailures(t *testing.T) {
	tx := &fakeTx{fakeSession: &fakeSession{}, opts: dal.NewTransactionOptions()}
	rawBackend := &fakeDB{fakeSession: &fakeSession{}, ro: tx, rw: tx}
	raw := dal.NewDB(rawBackend)
	boundRaw := BindDB(raw, context.Background())
	if _, ok := boundRaw.(*securedDB); !ok {
		t.Fatalf("raw bind=%T", boundRaw)
	}
	read := &securedDB{DB: raw}
	if _, ok := BindDB(read, context.Background()).(*securedDB); !ok {
		t.Fatal("secured read bind changed type")
	}
	write := &securedWriteDB{securedDB: read, writer: rawBackend}
	if _, ok := BindDB(write, context.Background()).(*securedWriteDB); !ok {
		t.Fatal("secured write bind changed type")
	}
	boom := errors.New("provider")
	db, err := SecureDB(raw, WithDatabasePolicyProvider(func(context.Context) ([]Policy, error) { return nil, boom }))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RunReadonlyTransaction(context.Background(), func(context.Context, dal.ReadTransaction) error { return nil }); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("readonly err=%v", err)
	}
	if err := db.RunReadwriteTransaction(context.Background(), func(context.Context, dal.ReadwriteTransaction) error { return nil }); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("readwrite err=%v", err)
	}
}
