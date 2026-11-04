package dal

import (
	"testing"
	"time"
)

func TestTimePrecisionIsMicrosecond(t *testing.T) {
	if TimePrecision != time.Microsecond {
		t.Fatalf("TimePrecision = %v, want %v", TimePrecision, time.Microsecond)
	}
}

func TestNormalizeTime(t *testing.T) {
	loc := time.FixedZone("UTC+2", 2*60*60)
	in := time.Date(2026, 11, 4, 10, 11, 12, 123456789, loc)
	got := NormalizeTime(in)
	want := time.Date(2026, 11, 4, 10, 11, 12, 123456000, loc)
	if got != want {
		t.Fatalf("NormalizeTime(%v) = %v, want %v", in, got, want)
	}
	if got.Location() != loc {
		t.Errorf("location = %v, want %v", got.Location(), loc)
	}
	if NormalizeTime(got) != got {
		t.Error("NormalizeTime must be idempotent")
	}

	now := time.Now()
	normalized := NormalizeTime(now)
	if normalized != NormalizeTime(now.Round(0)) {
		t.Error("NormalizeTime must strip the monotonic clock reading")
	}
	if normalized.Nanosecond()%1000 != 0 {
		t.Errorf("nanoseconds %d not a whole microsecond", normalized.Nanosecond())
	}
}
