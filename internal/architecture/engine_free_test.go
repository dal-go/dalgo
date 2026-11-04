// Package architecture holds tests that keep DALgo core's boundaries: see AGENTS.md,
// "DALgo core is engine-free: engine code lives in dalgo2<engine> adapters".
package architecture

import (
	"bufio"
	"go/scanner"
	"go/token"
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

// driverModules are module paths (or path prefixes) of database drivers and engine clients.
// They belong in dal-go/dalgo2<engine> repositories, never in DALgo core.
var driverModules = []string{
	"github.com/jackc/pgx",
	"github.com/jackc/pgconn",
	"github.com/lib/pq",
	"github.com/go-sql-driver/mysql",
	"github.com/mattn/go-sqlite3",
	"github.com/ncruces/go-sqlite3",
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
	"gorm.io/driver",
}

// engineCatalog matches engine-specific catalog names and introspection statements:
// Postgres pg_* catalogs, the SQL-standard information_schema, SQLite's master table and
// PRAGMAs, SQL Server sys.*, MySQL's grant tables, and Oracle's dba_* and v$ views.
var engineCatalog = regexp.MustCompile(`(?i)\b(pg_[a-z_]+|information_schema|sqlite_master|sqlite_schema|sys\.[a-z_]+|mysql\.(user|db|tables_priv|columns_priv|procs_priv|role_edges)|dba_[a-z_]+|PRAGMA\s+[a-z_]+)\b|\bv\$[a-z_]+`)

// isDriverModule reports whether a go.mod module path is a known database driver.
func isDriverModule(module string) bool {
	for _, d := range driverModules {
		if module == d || strings.HasPrefix(module, d+"/") {
			return true
		}
	}
	return false
}

// skipDir reports directories whose content is not part of DALgo core's own code.
func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "testdata":
		return true
	}
	return false
}

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

// requiredModules returns the module paths a go.mod requires, one per require line.
func requiredModules(goMod string) []string {
	var modules []string
	inBlock := false
	sc := bufio.NewScanner(strings.NewReader(goMod))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		switch {
		case len(fields) == 0:
		case inBlock && fields[0] == ")":
			inBlock = false
		case inBlock:
			modules = append(modules, fields[0])
		case fields[0] == "require" && len(fields) > 1 && fields[1] == "(":
			inBlock = true
		case fields[0] == "require" && len(fields) > 1:
			modules = append(modules, fields[1])
		}
	}
	return modules
}

func TestGoModRequiresNoDatabaseDriver(t *testing.T) {
	// Every module in the repository except testdata fixtures.
	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(moduleRoot, path)
		for _, m := range requiredModules(string(content)) {
			if isDriverModule(m) {
				t.Errorf("%s requires %s, a database driver: it belongs in a dal-go/dalgo2<engine> repository (AGENTS.md)", rel, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk module: %v", err)
	}
}

// goCodeOnly returns the Go source's tokens without its comments, so a doc comment may name
// a catalog as an example while every identifier and string literal (raw strings included)
// is still scanned.
func goCodeOnly(src []byte) string {
	fset := token.NewFileSet()
	file := fset.AddFile("src.go", -1, len(src))
	var s scanner.Scanner
	s.Init(file, src, nil, 0) // mode 0: comments are skipped
	var b strings.Builder
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			return b.String()
		}
		if lit == "" {
			lit = tok.String()
		}
		b.WriteString(lit)
		b.WriteByte(' ')
	}
}

func TestNonTestCodeHasNoEngineSQLOrDatabaseSQL(t *testing.T) {
	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		var code string
		switch ext := filepath.Ext(path); {
		case ext == ".go" && !strings.HasSuffix(path, "_test.go"):
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			code = goCodeOnly(content)
		case ext == ".sql" || ext == ".c" || ext == ".h":
			// Embedded SQL and cgo sources are engine code in full, comments included.
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			code = string(content)
		default:
			return nil
		}
		rel, _ := filepath.Rel(moduleRoot, path)
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
	// Positive controls, so a broken pattern, filter or parser cannot make the scans pass vacuously.
	for _, s := range []string{
		"SELECT * FROM pg_catalog.pg_roles", "select * from pg_tables", "pg_stat_activity",
		"from INFORMATION_SCHEMA.TABLES", "sqlite_master", "PRAGMA table_info(t)",
		"sys.objects", "sys.server_principals", "mysql.user", "DBA_ROLE_PRIVS", "v$session",
	} {
		if !engineCatalog.MatchString(s) {
			t.Errorf("pattern misses %q", s)
		}
	}
	for _, s := range []string{"mypg_roles", "my_information_schemas", "system.objects", "mysqlx.user", "nav$x"} {
		if engineCatalog.MatchString(s) {
			t.Errorf("pattern wrongly matches %q", s)
		}
	}
	src := []byte("package x\n// information_schema is an example\n/* pg_roles\n also an example */\n/* c */ const q = `SELECT 1 FROM pg_roles`\n")
	if got := engineCatalog.FindString(goCodeOnly(src)); got != "pg_roles" {
		t.Errorf("code-only scan found %q, want the pg_roles in code and none from comments", got)
	}
	if engineCatalog.MatchString(goCodeOnly([]byte("package x\n// information_schema only in a comment\n"))) {
		t.Error("a doc comment must not count as code")
	}
	if !strings.Contains(goCodeOnly([]byte("package x\nimport \"database/sql\"\n")), `"database/sql"`) {
		t.Error("the database/sql import check cannot see imports")
	}
	mods := requiredModules("module m\n\nrequire github.com/jackc/pgx/v5 v5.7.1\nrequire (\n\tgorm.io/driver/postgres v1.5.0 // indirect\n\tgithub.com/strongo/random v0.0.1\n)\n")
	var drivers []string
	for _, m := range mods {
		if isDriverModule(m) {
			drivers = append(drivers, m)
		}
	}
	if strings.Join(drivers, ",") != "github.com/jackc/pgx/v5,gorm.io/driver/postgres" {
		t.Errorf("driver check found %v in %v, want pgx and the gorm driver", drivers, mods)
	}
	if isDriverModule("github.com/lib/pqx") {
		t.Error("driver check must match whole path segments")
	}
}
