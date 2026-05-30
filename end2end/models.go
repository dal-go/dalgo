package end2end

import (
	"fmt"
	"strings"

	"github.com/strongo/validation"
)

const (
	TestEntitiesNamePrefix = "DalgoE2E_"

	// E2ETestKind1 defines table or collection name for an entity to be stored in
	E2ETestKind1 = TestEntitiesNamePrefix + "E2ETest1"
	// E2ETestKind2 defines table or collection name for an entity to be stored in
	E2ETestKind2 = TestEntitiesNamePrefix + "E2ETest2"

	//UserKind = TestEntitiesNamePrefix + "User"
)

type User struct {
	Title string `json:"title,omitempty"`
	Email string `json:"email,omitempty"`
}

// TestData describes a test entity to be stored in a DALgo database
type TestData struct {
	StringProp  string `json:"StringProp,omitempty" db:"StringProp"`
	IntegerProp int    `json:"IntegerProp" db:"IntegerProp"`
}

// Validate returns error if not valid
func (v TestData) Validate() error {
	if strings.TrimSpace(v.StringProp) == "" {
		return validation.NewErrRecordIsMissingRequiredField("StringProp")
	}
	if v.IntegerProp < 0 {
		return validation.NewErrBadRecordFieldValue("IntegerProp", fmt.Sprintf("should be > 0, got: %v", v.IntegerProp))
	}
	return nil
}
