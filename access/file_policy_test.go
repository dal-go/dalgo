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
	policies, err := LoadPolicyFiles(root, FilePolicyConfig{Enabled: true, Database: "db1", Policies: []string{"policy.yaml"}})
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.Equal(t, "policy-one", policies[0].Name())
	assert.Equal(t, "policy.yaml", policies[0].(*AccessPolicy).Source())
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
		"field mask":       {body: portablePolicy("p", "public", `scopes: [{path: /cities/*, rules: [{id: read, effect: allow, operations: [get], fieldMask: {stages: [{include: ['*']}]}}]}]`), database: "db1", want: "fieldMask is not supported"},
		"null field mask":  {body: portablePolicy("p", "public", `scopes: [{path: /cities/*, rules: [{id: read, effect: allow, operations: [get], fieldMask: null}]}]`), database: "db1", want: "fieldMask is not supported"},
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
