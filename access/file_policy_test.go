package access

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPolicyFilesDisabled(t *testing.T) {
	policies, err := LoadPolicyFiles("", FilePolicyConfig{})
	require.NoError(t, err)
	assert.Nil(t, policies)
}

func TestLoadPolicyFiles(t *testing.T) {
	root := t.TempDir()
	writePolicyFile(t, root, "policy.yaml", portablePolicy("policy-one", "public", `scopes:
  - path: /cities/*
    rules:
      - id: read
        effect: allow
        operations: [get]
`))
	policies, err := LoadPolicyFiles(root, FilePolicyConfig{Enabled: true, Database: "db1", Realm: "people", Policies: []string{"policy.yaml"}})
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.Equal(t, "policy-one", policies[0].Name())
	assert.Equal(t, "policy.yaml", policies[0].(*AccessPolicy).Source())
	assert.Equal(t, "people", policies[0].(*AccessPolicy).realm)
	metadata := DescribePolicy(policies[0])
	assert.Equal(t, PolicyMetadata{ID: "policy-one", Revision: metadata.Revision, Visibility: PolicyVisibilityPublic, Source: "policy.yaml"}, metadata)
	assert.True(t, strings.HasPrefix(metadata.Revision, "sha256:"))
	for _, realm := range []string{" people", "people\x00", string([]byte{0xff})} {
		_, err := LoadPolicyFiles(root, FilePolicyConfig{Enabled: true, Database: "db1", Realm: realm, Policies: []string{"policy.yaml"}})
		require.ErrorContains(t, err, "policy realm")
	}
}

func TestLoadPolicyFilesPrincipalBindings(t *testing.T) {
	root := t.TempDir()
	writePolicyFile(t, root, "bindings.json", `{
  "apiVersion":"dtql.org/access/v1", "kind":"AccessPolicy",
  "metadata":{"name":"bound"}, "target":{"database":"db1"},
  "composition":"dalgo-hierarchical-v1", "default":"deny",
  "ruleSets":{"reader":[{"path":"/cities/*","rules":[{"id":"read","effect":"allow","operations":["get"]}]}]},
  "bindings":{"everyone":["reader"]}
}`)
	policies, err := LoadPolicyFiles(root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{"bindings.json"}})
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.IsType(t, &PrincipalPolicySet{}, policies[0])
}

func TestLoadPolicyFilesFailsClosed(t *testing.T) {
	tests := map[string]struct{ body, database, want string }{
		"empty list":       {database: "db1", want: "at least one"},
		"missing":          {body: "missing", database: "db1", want: "inspect policy file"},
		"wrong target":     {body: portablePolicy("p", "public", validPortableScopes), database: "other", want: "does not match"},
		"unknown field":    {body: portablePolicy("p", "public", validPortableScopes+"unknown: true\n"), database: "db1", want: "field unknown"},
		"bad visibility":   {body: portablePolicy("p", "secret", validPortableScopes), database: "db1", want: "visibility"},
		"private accepted": {body: portablePolicy("p", "private", validPortableScopes), database: "db1"},
		"field mask":       {body: portablePolicy("p", "public", `scopes: [{path: /cities/*, rules: [{id: read, effect: allow, operations: [get], fieldMask: {stages: [{include: ['*']}]}}]}]`), database: "db1"},
		"null field mask":  {body: portablePolicy("p", "public", `scopes: [{path: /cities/*, rules: [{id: read, effect: allow, operations: [get], fieldMask: null}]}]`), database: "db1", want: "mask requires an object"},
		"opaque scope":     {body: portablePolicy("p", "public", `scopes: [{opaqueQuery: true, rules: [{id: read, effect: allow, operations: [query]}]}]`), database: "db1", want: "path scopes only"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			files := []string(nil)
			if test.body != "" {
				files = []string{"policy.yaml"}
				if test.body != "missing" {
					writePolicyFile(t, root, files[0], test.body)
				}
			}
			policies, err := LoadPolicyFiles(root, FilePolicyConfig{Enabled: true, Database: test.database, Policies: files})
			if test.want == "" {
				require.NoError(t, err)
				require.Len(t, policies, 1)
				return
			}
			require.ErrorContains(t, err, test.want)
			assert.Nil(t, policies)
		})
	}
}

func TestLoadPolicyFilesRejectsDuplicateIDsAndEscapes(t *testing.T) {
	root := t.TempDir()
	writePolicyFile(t, root, "one.yaml", portablePolicy("same", "public", validPortableScopes))
	writePolicyFile(t, root, "two.yaml", portablePolicy("same", "public", validPortableScopes))
	_, err := LoadPolicyFiles(root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{"one.yaml", "two.yaml"}})
	require.ErrorContains(t, err, `duplicate policy id "same"`)

	outside := filepath.Join(t.TempDir(), "outside.yaml")
	require.NoError(t, os.WriteFile(outside, []byte(portablePolicy("outside", "public", validPortableScopes)), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "link.yaml")))
	_, err = LoadPolicyFiles(root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{"link.yaml"}})
	require.Error(t, err)
}

func TestLoadPolicyFilesRejectsAmbiguousYAML(t *testing.T) {
	root := t.TempDir()
	tests := map[string]string{
		"duplicate.json": `{"apiVersion":"dtql.org/access/v1","apiVersion":"dtql.org/access/v1"}`,
		"alias.yaml":     "common: &common {name: p}\nmetadata: *common\n",
		"merge.yaml":     "common: &common {name: p}\nmetadata:\n  <<: *common\n",
	}
	for name, body := range tests {
		writePolicyFile(t, root, name, body)
		_, err := LoadPolicyFiles(root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{name}})
		require.Error(t, err, name)
	}

	// A principal identifier named like an unsupported top-level feature is data,
	// not an execution gate, and reaches ordinary binding validation.
	writePolicyFile(t, root, "binding.yaml", portablePolicy("bound", "public", `ruleSets:
  reader:
    - path: /cities/*
      rules: [{id: read, effect: allow, operations: [get]}]
bindings:
  users:
    execution: [reader]
`))
	policies, err := LoadPolicyFiles(root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{"binding.yaml"}})
	require.NoError(t, err)
	require.Len(t, policies, 1)

	require.NoError(t, os.Mkdir(filepath.Join(root, "directory.yaml"), 0o700))
	_, err = LoadPolicyFiles(root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{"directory.yaml"}})
	require.ErrorContains(t, err, "not a regular file")
}

func TestLoadPolicyFilesRejectsInvalidConfigurationAndFiles(t *testing.T) {
	root := t.TempDir()
	writePolicyFile(t, root, "policy.txt", portablePolicy("p", "public", validPortableScopes))
	writePolicyFile(t, root, "bad.json", "{")
	require.NoError(t, os.WriteFile(filepath.Join(root, "large.yaml"), make([]byte, maxPolicyFileBytes+1), 0o600))
	tests := []struct {
		name   string
		root   string
		config FilePolicyConfig
		want   string
	}{
		{"database", root, FilePolicyConfig{Enabled: true, Policies: []string{"policy.txt"}}, "require a database"},
		{"root", filepath.Join(root, "missing"), FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{"policy.yaml"}}, "open policy root"},
		{"empty name", root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{""}}, "invalid policy file"},
		{"absolute", root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{filepath.Join(root, "policy.txt")}}, "invalid policy file"},
		{"extension", root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{"policy.txt"}}, "must use"},
		{"json", root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{"bad.json"}}, "invalid JSON"},
		{"size", root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{"large.yaml"}}, "exceeds"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := LoadPolicyFiles(tc.root, tc.config); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want=%q", err, tc.want)
			}
		})
	}
}

func TestPortablePolicySyntaxAndEnvelopeValidation(t *testing.T) {
	for name, body := range map[string]string{
		"syntax delegated": "bad: [",
		"empty":            "",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validatePortablePolicyYAML([]byte(body)); err != nil {
				t.Fatalf("syntax precheck must defer decoder errors: %v", err)
			}
		})
	}
	for name, body := range map[string]string{
		"execution":       "execution: {}",
		"collection mask": "collectionMask: {}",
		"scope mask":      "scopes: [{path: /x/*, collectionMask: {}}]",
		"rule mask":       "scopes: [{path: /x/*, rules: [{fieldMask: {}}]}]",
		"nested mask":     "scopes: [{path: /x/*, scopes: [{path: /y/*, collectionMask: {}}]}]",
		"ruleset mask":    "ruleSets: {r: [{path: /x/*, rules: [{fieldMask: {}}]}]}",
		"duplicate":       "metadata: {name: a, name: b}",
		"alias":           "x: &x {name: a}\nmetadata: *x",
		"merge":           "x: &x {name: a}\nmetadata: {<<: *x}",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validatePortablePolicyYAML([]byte(body)); err == nil {
				t.Fatal("unsupported or ambiguous syntax accepted")
			}
		})
	}
	if got := mappingFeature(nil, "x"); got != "" {
		t.Fatalf("nil mapping feature=%q", got)
	}
}

func TestPolicyFromDTQLDocumentRejectsUnsupportedEnvelope(t *testing.T) {
	source := []byte(portablePolicy("p", "public", validPortableScopes))
	tests := map[string]func(DTQLDocument) DTQLDocument{
		"api":         func(d DTQLDocument) DTQLDocument { d.APIVersion = "future"; return d },
		"visibility":  func(d DTQLDocument) DTQLDocument { d.Metadata.Visibility = "secret"; return d },
		"target":      func(d DTQLDocument) DTQLDocument { d.Target.Database = "other"; return d },
		"composition": func(d DTQLDocument) DTQLDocument { d.Composition = "future"; return d },
		"opaque":      func(d DTQLDocument) DTQLDocument { d.Scopes[0].OpaqueQuery = true; return d },
		"group":       func(d DTQLDocument) DTQLDocument { d.Scopes[0].CollectionGroup = "g"; return d },
		"scope mask":  func(d DTQLDocument) DTQLDocument { d.Scopes[0].CollectionMask = &Mask{}; return d },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			base, err := ParseDTQLPolicy(source)
			require.NoError(t, err)
			if _, err := policyFromDTQLDocument(mutate(base), "db1", "policy.yaml"); err == nil {
				t.Fatal("unsupported envelope accepted")
			}
		})
	}
}

const validPortableScopes = `scopes:
  - path: /cities/*
    rules: [{id: read, effect: allow, operations: [get]}]
`

func portablePolicy(name, visibility, body string) string {
	return "apiVersion: dtql.org/access/v1\nkind: AccessPolicy\nmetadata:\n  name: " + name + "\n  visibility: " + visibility + "\ntarget:\n  database: db1\ncomposition: dalgo-hierarchical-v1\ndefault: deny\n" + body
}

func writePolicyFile(t *testing.T, root, name, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(strings.TrimSpace(body)+"\n"), 0o600))
}
