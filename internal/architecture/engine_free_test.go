// Package architecture holds tests that keep DALgo core's boundaries: see AGENTS.md,
// "DALgo core is engine-free: engine code lives in dalgo2<engine> adapters".
package architecture

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// moduleRoot is the repository root, relative to this package.
const moduleRoot = "../.."

// allowedAdapters are the only adapters DALgo core may hold (AGENTS.md).
var allowedAdapters = []string{"dalgo2fs", "dalgo2memory"}

// driverModules are module path prefixes of database drivers and engine clients.
// They belong in dal-go/dalgo2<engine> repositories, never in DALgo core.
var driverModules = []string{
	"github.com/jackc/pgx",
	"github.com/lib/pq",
	"github.com/go-sql-driver/mysql",
	"github.com/mattn/go-sqlite3",
	"modernc.org/sqlite",
	"github.com/microsoft/go-mssqldb",
	"github.com/denisenkom/go-mssqldb",
	"go.mongodb.org/mongo-driver",
	"cloud.google.com/go/firestore",
	"cloud.google.com/go/bigquery",
	"cloud.google.com/go/spanner",
	"github.com/ClickHouse/clickhouse-go",
	"github.com/snowflakedb/gosnowflake",
	"github.com/sijms/go-ora",
	"github.com/godror/godror",
	"github.com/redis/go-redis",
	"github.com/elastic/go-elasticsearch",
	"github.com/duckdb/duckdb-go",
	"github.com/marcboeker/go-duckdb",
	"github.com/tursodatabase/libsql-client-go",
}

// engineCatalog matches engine-specific catalog names.
var engineCatalog = regexp.MustCompile(`(?i)\b(pg_catalog|pg_roles|pg_authid|pg_auth_members|pg_class|pg_namespace|information_schema|sqlite_master|sqlite_schema|sys\.objects|sys\.tables|sys\.database_principals)\b`)

func TestOnlyAllowedAdaptersInCore(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(moduleRoot, "adapters"))
	if err != nil {
		t.Fatalf("read adapters: %v", err)
	}
	var got []string
	for _, e := range entries {
		if e.IsDir() {
			got = append(got, e.Name())
		}
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(allowedAdapters, ",") {
		t.Errorf("adapters/ holds %v; DALgo core may hold only %v: a new adapter belongs in its own dal-go/dalgo2<engine> repository (AGENTS.md)", got, allowedAdapters)
	}
}

func TestGoModRequiresNoDatabaseDriver(t *testing.T) {
	f, err := os.Open(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		t.Fatalf("open go.mod: %v", err)
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "require ")
		for _, module := range driverModules {
			if strings.HasPrefix(text, module) {
				t.Errorf("go.mod:%d requires %s, a database driver: it belongs in a dal-go/dalgo2<engine> repository (AGENTS.md)", line, module)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
}

// codeOnly drops line comments and block comments, so a doc comment may name a catalog as an example.
func codeOnly(src string) string {
	var b strings.Builder
	inBlock := false
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if inBlock {
			if i := strings.Index(line, "*/"); i >= 0 {
				inBlock = false
				line = line[i+2:]
			} else {
				continue
			}
		} else if strings.HasPrefix(trimmed, "/*") {
			if !strings.Contains(trimmed, "*/") {
				inBlock = true
			}
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestNonTestCodeHasNoEngineSQLOrDatabaseSQL(t *testing.T) {
	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(moduleRoot, path)
		code := codeOnly(string(content))
		if strings.Contains(code, `"database/sql"`) {
			t.Errorf("%s imports database/sql: engine access belongs in a dal-go/dalgo2<engine> repository (AGENTS.md)", rel)
		}
		if m := engineCatalog.FindString(code); m != "" {
			t.Errorf("%s contains %q in code, an engine catalog name: engine-specific SQL belongs in a dal-go/dalgo2<engine> repository (AGENTS.md)", rel, m)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk module: %v", err)
	}
}

func TestCheckersCatchKnownViolations(t *testing.T) {
	// Positive controls, so a broken pattern or comment filter cannot make the scans pass vacuously.
	for _, s := range []string{"SELECT * FROM pg_catalog.pg_roles", "from INFORMATION_SCHEMA.TABLES", "sqlite_master", "sys.objects"} {
		if !engineCatalog.MatchString(s) {
			t.Errorf("pattern misses %q", s)
		}
	}
	for _, s := range []string{"pg_roleset", "my_information_schemas", "system.objects"} {
		if engineCatalog.MatchString(s) {
			t.Errorf("pattern wrongly matches %q", s)
		}
	}
	src := "package x\n// information_schema is an example\n/* pg_roles\n also an example */\nconst q = \"SELECT 1 FROM pg_roles\"\n"
	if got := engineCatalog.FindString(codeOnly(src)); got != "pg_roles" {
		t.Errorf("code-only scan found %q, want the pg_roles in code and none from comments", got)
	}
	if engineCatalog.MatchString(codeOnly("// information_schema only in a comment\n")) {
		t.Error("a doc comment must not count as code")
	}
}
