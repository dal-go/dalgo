package dal

import "time"

// TimePrecision is the finest precision at which DALgo guarantees a
// time.Time value survives a write and a read: one microsecond.
//
// Adapters persist time.Time values at no better than this precision. Common
// production stores keep timestamps in microseconds (Firestore and Postgres
// both do), so a value written with nanoseconds comes back truncated to the
// microsecond. The in-memory adapter enforces the same contract, so unit
// tests observe exactly what production storage returns.
//
// Callers must not rely on sub-microsecond precision surviving a round trip.
// A time that is also kept elsewhere — in a hash, a JSON payload, a cache or
// a comparison key — must be normalised with [NormalizeTime] before it is
// both stored and kept, so the two copies stay equal.
const TimePrecision = time.Microsecond

// NormalizeTime returns t truncated to [TimePrecision], the precision every
// adapter guarantees to preserve. Like time.Time.Truncate, it also strips any
// monotonic clock reading, so the result compares with == and Equal the same
// way a value read back from storage does. The location is kept.
func NormalizeTime(t time.Time) time.Time {
	return t.Truncate(TimePrecision)
}
