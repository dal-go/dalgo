package dtql

import "testing"

// canonicalDocs are valid in-scope DTQL-YAML documents already in canonical form
// (stable key order and 2-space indentation, as emitted by Serialize).
var canonicalDocs = map[string]string{
	"comparison with in array": `from:
  name: users
columns:
  - field: name
  - field: age
    as: years
where:
  op: In
  left:
    field: status
  right:
    values:
      - active
      - pending
orderBy:
  - field: name
  - field: created
    desc: true
limit: 10
offset: 20
`,
	"nested and/or groups": `from:
  name: users
columns:
  - field: name
  - field: age
    as: years
where:
  and:
    - op: '>='
      left:
        field: age
      right:
        value: 18
    - or:
        - op: In
          left:
            field: status
          right:
            values:
              - active
              - pending
        - op: ==
          left:
            field: country
          right:
            value: US
orderBy:
  - field: name
  - field: age
    desc: true
limit: 10
offset: 20
`,
}

// TestCanonicalRoundTrip asserts serialize(deserialize(d)) is byte-identical to a
// valid canonical document d, so saved queries diff cleanly across edits.
func TestCanonicalRoundTrip(t *testing.T) {
	for name, d := range canonicalDocs {
		t.Run(name, func(t *testing.T) {
			q, err := Deserialize([]byte(d))
			if err != nil {
				t.Fatalf("Deserialize: %v", err)
			}
			got, err := Serialize(q)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			if string(got) != d {
				t.Fatalf("serialize(deserialize(d)) not byte-identical to d.\n--- want ---\n%s\n--- got ---\n%s", d, got)
			}
		})
	}
}

// TestSerializeIsIdempotent asserts the serializer is canonical for any in-scope
// query: serializing a deserialized serialization reproduces the same bytes.
func TestSerializeIsIdempotent(t *testing.T) {
	for name, q := range roundTripCases() {
		t.Run(name, func(t *testing.T) {
			s1, err := Serialize(q)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			q2, err := Deserialize(s1)
			if err != nil {
				t.Fatalf("Deserialize: %v", err)
			}
			s2, err := Serialize(q2)
			if err != nil {
				t.Fatalf("re-Serialize: %v", err)
			}
			if string(s1) != string(s2) {
				t.Fatalf("serialization is not idempotent.\n--- s1 ---\n%s\n--- s2 ---\n%s", s1, s2)
			}
		})
	}
}
