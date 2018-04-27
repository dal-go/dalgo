package gaedb

import (
	"github.com/matryer/is"
	"google.golang.org/appengine/datastore"
	"testing"
	"time"
)

func TestIsEmptyJson(t *testing.T) {
	t.Parallel()
	is := is.New(t)

	p := datastore.Property{
		Name:    "TestProp1",
		Value:   nil,
		NoIndex: true,
	}
	is.True(IsEmptyJSON(p)) // nil should be treated as empty string

	p.Value = ""
	is.True(IsEmptyJSON(p)) // Empty string should return true"

	p.Value = "[]"
	is.True(IsEmptyJSON(p)) // Empty string should return true"

	p.Value = "{}"
	is.True(IsEmptyJSON(p)) // Empty string should return true"

	p.Value = "0"
	is.True(!IsEmptyJSON(p)) // '0' string should return false"
}

func TestIsZeroTime(t *testing.T) {
	t.Parallel()
	is := is.New(t)

	p := datastore.Property{
		Name:    "TestProp1",
		Value:   nil,
		NoIndex: true,
	}
	is.True(IsZeroTime(p)) // nil should be treated as zero value

	p.Value = time.Time{}
	is.True(IsZeroTime(p)) // should return true for zero value

	p.Value = time.Now()
	is.True(!IsZeroTime(p)) //  string should return false for time.Now()
}
