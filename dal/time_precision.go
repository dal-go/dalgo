package dal

import "time"

// TimePrecision is a ceiling on time precision: no DALgo adapter keeps a
// time.Time more precisely than one microsecond, and some keep less (MongoDB's
// BSON datetime, for example, keeps milliseconds). Common production stores
// keep microseconds (Firestore and Postgres both do), so a value written with
// nanoseconds comes back truncated or rounded to at least the microsecond. The
// in-memory adapter truncates to exactly this precision, so unit tests see the
// loss production storage causes.
//
// Callers must not rely on sub-microsecond precision surviving a write and a
// read. A time that is also kept elsewhere — in a hash, a JSON payload, a
// cache or a comparison key — must be normalised to at least this precision
// (with [NormalizeTime], or coarser for an adapter that keeps less) before it
// is both stored and kept, so the two copies stay equal.
const TimePrecision = time.Microsecond

// NormalizeTime returns t truncated to [TimePrecision], the finest precision
// any adapter keeps. Like time.Time.Truncate, it also strips any
// monotonic clock reading, so the result compares with == and Equal the same
// way a value read back from storage does. The location is kept.
func NormalizeTime(t time.Time) time.Time {
	return t.Truncate(TimePrecision)
}
