package access

import (
	"context"
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
