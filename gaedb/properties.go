package gaedb

import (
	"fmt"
	"github.com/pkg/errors"
	"google.golang.org/appengine/datastore"
	"time"
)

// IsOkToRemove checks if it's OK to remove property value
type IsOkToRemove func(p datastore.Property) bool // TODO: Open source + article?

// IsObsolete used for obsolete properties
func IsObsolete(_ datastore.Property) bool {
	return true
}

// IsDuplicate indicates property is duplicate
func IsDuplicate(_ datastore.Property) bool {
	return true
}

// IsFalse returns true if propertry value is false
func IsFalse(p datastore.Property) bool {
	return p.Value == nil || !p.Value.(bool)
}

// IsZeroInt returns true if property value is 0
func IsZeroInt(p datastore.Property) bool {
	return p.Value == nil || p.Value.(int64) == 0
}

// IsZeroBool returns true if property value false
func IsZeroBool(p datastore.Property) bool {
	return p.Value == nil || !p.Value.(bool)
}

// IsZeroFloat returns true if property value 0.0
func IsZeroFloat(p datastore.Property) bool {
	return p.Value == nil || p.Value.(float64) == 0
}

// IsZeroTime returns true of property value iz zero time
func IsZeroTime(p datastore.Property) bool {
	return p.Value == nil || p.Value.(time.Time).IsZero()
}

// IsEmptyString returns true if property value iz empty string
func IsEmptyString(p datastore.Property) bool {
	return p.Value == nil || p.Value.(string) == "" // TODO: Do we need to check for nil?
}

// IsEmptyJSON returns true if property value is empty string or empty array or empty object
func IsEmptyJSON(p datastore.Property) bool {
	if p.Value == nil {
		return true
	}
	v := p.Value.(string)
	return v == "" || v == "{}" || v == "[]"
}

// IsEmptyByteArray returns true of property value is nil or empty byte array
func IsEmptyByteArray(p datastore.Property) bool {
	if p.Value == nil {
		return true
	}
	v := p.Value.([]uint8)
	return v == nil || len(v) == 0
}

// IsEmptyStringOrSpecificValue returns true if property value is empty string or has specific value
func IsEmptyStringOrSpecificValue(v string) func(p datastore.Property) bool {
	return func(p datastore.Property) bool {
		if p.Value == nil {
			return true
		}
		s := p.Value.(string)
		return s == "" || s == v
	}
}

// CleanProperties removes properties in place and returns filtered slice
func CleanProperties(properties []datastore.Property, filters map[string]IsOkToRemove) (filtered []datastore.Property, err error) {
	var (
		i int
		p datastore.Property
	)

	if properties == nil {
		return properties, errors.New("properties == nil")
	}

	if filters == nil {
		return properties, errors.New("filters == nil")
	}

	if len(filters) == 0 {
		return properties, errors.New("len(filters) == 0")
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("failed to process property %v=%T(%v), recovered: %v", p.Name, p.Value, p.Value, r)
		}
		filtered = properties[:i]
	}()

	for _, p = range properties {
		if filter, ok := filters[p.Name]; !ok || !filter(p) {
			properties[i] = p
			i++
		}
	}
	return
}
