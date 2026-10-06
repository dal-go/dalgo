// Package datarights carries optional provider-declared source data terms.
// It does not evaluate terms, grant access, resolve inheritance, or assign a
// licence to query output. Providers own those policies and evidence validation.
package datarights

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Declaration describes source terms. An absent declaration is not permission.
type Declaration struct {
	Name string `json:"name,omitempty"`
	SPDX string `json:"spdx,omitempty"`
	URL  string `json:"url,omitempty"`
	Text string `json:"text,omitempty"`
}

// Source identifies the provider's exact source. IDs are opaque to DALgo.
type Source struct {
	ServerID   string `json:"serverId"`
	DatabaseID string `json:"databaseId,omitempty"`
	Recordset  string `json:"recordset,omitempty"`
}

// Pin identifies immutable evidence where a provider has verified it.
// Role is provider-defined (for example provider, declaration, input or terms).
type Pin struct {
	Role       string `json:"role"`
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	Bytes      int64  `json:"bytes"`
}

// Notice is descriptive credit or a source link supplied by the provider.
type Notice struct {
	Text string `json:"text"`
	URL  string `json:"url,omitempty"`
}

// LinkNotice is a free-source notice with a required canonical HTTPS link.
// Providers validate URL safety and evidence before exposing the metadata.
type LinkNotice struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// SourceRight describes effective source terms and their authored scope.
// Scope and EvidenceOrigin are provider-defined; conventional scopes are
// server, database and recordset. Pins may be empty for unpinned declarations.
// Neither SPDX nor EvidenceOrigin is a DALgo certification of rights.
type SourceRight struct {
	SourceID         string      `json:"sourceId"`
	Source           Source      `json:"source"`
	Declaration      Declaration `json:"declaration"`
	DeclarationScope string      `json:"declarationScope"`
	DeclaredAt       Source      `json:"declaredAt"`
	EvidenceOrigin   string      `json:"evidenceOrigin"`
	Pins             []Pin       `json:"pins"`
	Attribution      *Notice     `json:"attribution,omitempty"`
	FreeSource       *LinkNotice `json:"freeSource,omitempty"`
	Transformations  []string    `json:"transformations"`
}

// MarshalJSON emits required evidence arrays as [] even for unpinned Go values.
// Optional top-level inventories are handled separately by QueryMetadata.
func (r SourceRight) MarshalJSON() ([]byte, error) {
	type wire SourceRight
	w := wire(r)
	if w.Pins == nil {
		w.Pins = []Pin{}
	}
	if w.Transformations == nil {
		w.Transformations = []string{}
	}
	return json.Marshal(w)
}

// UnmarshalJSON rejects null or missing required evidence arrays. This is wire
// shape validation only; provider authority and legal/evidence checks are separate.
func (r *SourceRight) UnmarshalJSON(data []byte) error {
	if err := checkArrayFields(data, true, "pins", "transformations"); err != nil {
		return err
	}
	type wire SourceRight
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*r = SourceRight(w)
	return nil
}

// QueryMetadata is an optional inventory of source data terms, not output terms.
// SourceRights contains the authorized planned inventory captured before output.
// UsedSourceIDs identifies inputs actually read or considered, including empty
// and projected-away inputs. Providers must not infer it from output rows.
// Omission means unknown/not provided. Providers own ordering, identity,
// authorization, preflight, evidence budgets and page snapshot consistency.
type QueryMetadata struct {
	SourceRights  []SourceRight `json:"sourceRights,omitempty"`
	UsedSourceIDs []string      `json:"usedSourceIds,omitempty"`
}

// MarshalJSON preserves the distinction between omitted and known-empty
// inventories. This matches optional query-page arrays in the JS contract.
func (m QueryMetadata) MarshalJSON() ([]byte, error) {
	type wire struct {
		SourceRights  *[]SourceRight `json:"sourceRights,omitempty"`
		UsedSourceIDs *[]string      `json:"usedSourceIds,omitempty"`
	}
	var w wire
	if m.SourceRights != nil {
		w.SourceRights = &m.SourceRights
	}
	if m.UsedSourceIDs != nil {
		w.UsedSourceIDs = &m.UsedSourceIDs
	}
	return json.Marshal(w)
}

// UnmarshalJSON accepts omitted inventories and empty arrays, but rejects
// explicit null arrays, matching the JS query-page contract.
func (m *QueryMetadata) UnmarshalJSON(data []byte) error {
	if err := checkArrayFields(data, false, "sourceRights", "usedSourceIds"); err != nil {
		return err
	}
	type wire QueryMetadata
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*m = QueryMetadata(w)
	return nil
}

func checkArrayFields(data []byte, required bool, names ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("source rights metadata must be an object")
	}
	for _, name := range names {
		raw := bytes.TrimSpace(fields[name])
		if (required && len(raw) == 0) || bytes.Equal(raw, []byte("null")) {
			return fmt.Errorf("source rights field %s must be an array", name)
		}
	}
	return nil
}

// Clone returns a detached snapshot safe from subsequent caller mutations.
func (m QueryMetadata) Clone() QueryMetadata {
	c := QueryMetadata{SourceRights: cloneSlice(m.SourceRights), UsedSourceIDs: cloneSlice(m.UsedSourceIDs)}
	for i := range c.SourceRights {
		r := &c.SourceRights[i]
		r.Pins = cloneSlice(r.Pins)
		r.Transformations = cloneSlice(r.Transformations)
		if r.Attribution != nil {
			n := *r.Attribution
			r.Attribution = &n
		}
		if r.FreeSource != nil {
			n := *r.FreeSource
			r.FreeSource = &n
		}
	}
	return c
}

func cloneSlice[T any](v []T) []T {
	if v == nil {
		return nil
	}
	c := make([]T, len(v))
	copy(c, v)
	return c
}
