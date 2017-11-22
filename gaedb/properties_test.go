package gaedb

import (
	"testing"
	"google.golang.org/appengine/datastore"
	isLib "github.com/matryer/is"
)

func TestIsEmptyString(t *testing.T) {
	is := isLib.New(t)

	p := datastore.Property{
		Name: "TestProp1",
		Value: nil,
		NoIndex: true,
	}
	is.True(IsEmptyString(p)) // nil should be treated as empty string

	p.Value = ""
	is.True(IsEmptyString(p)) // Empty string should return true"

	p.Value = "0"
	is.True(!IsEmptyString(p))  // '0' string should return false"
}
