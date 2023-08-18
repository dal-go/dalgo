package dal

import (
	"errors"
	"strings"
)

// FieldVal hold a reference to a single record within a root or nested recordset.
type FieldVal struct {
	Name  string `json:"Name"`
	Value any    `json:"value"`
}

// Validate validates field value
func (v FieldVal) Validate() error {
	if strings.TrimSpace(v.Name) == "" {
		return errors.New("missing field name")
	}
	return nil
}
