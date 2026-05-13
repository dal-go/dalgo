package dbschema

// DefaultExpr is the sealed interface for column default value
// expressions. The interface is sealed via an unexported marker method
// (defaultExpr) so the set of valid DefaultExpr cases is closed at the
// package boundary — drivers know which cases exist and translate
// accordingly. New cases require adding to this package.
//
// MVP concretes:
//   - [DefaultLiteral] carries a plain Go value
//   - [DefaultCurrentTimestamp] signals "use the engine's current
//     timestamp default at insertion time"
type DefaultExpr interface {
	defaultExpr() // sealed marker
}

// DefaultLiteral is a column default that uses a literal Go value.
// Drivers MUST handle at minimum: int, int64, float64, string, bool,
// []byte, and nil. Drivers MAY return a driver-specific error if the
// underlying Go type cannot be serialized to the target engine.
type DefaultLiteral struct {
	Value any
}

func (DefaultLiteral) defaultExpr() {}

// DefaultCurrentTimestamp signals "use the engine's current-timestamp
// default at insertion time." Drivers translate to engine-specific
// SQL (CURRENT_TIMESTAMP on SQLite, now() or CURRENT_TIMESTAMP on
// PostgreSQL) or the equivalent.
//
// Cross-engine precision: SQLite returns timestamps at second
// resolution by default; PostgreSQL at microsecond. The Feature does
// not pin precision — drivers translate as their engine allows.
type DefaultCurrentTimestamp struct{}

func (DefaultCurrentTimestamp) defaultExpr() {}
